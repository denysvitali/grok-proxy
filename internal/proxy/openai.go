package proxy

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/denysvitali/grok-proxy/internal/grok"
	"github.com/denysvitali/grok-proxy/internal/openai"
)

func (s *Server) responses(w http.ResponseWriter, request *http.Request) {
	var input openai.ResponsesRequest
	if err := s.decodeRequest(w, request, &input); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON request")
		return
	}
	if input.Model == "" {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if err := validateOpenAIInput(input.Input); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}

	requestedModel := input.Model
	resolvedModel := s.config.ResolveModel(requestedModel)
	input.Model = resolvedModel
	body, err := json.Marshal(input)
	if err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid_request_error", "request could not be encoded")
		return
	}

	accept := "application/json"
	if input.Stream {
		accept = "text/event-stream"
	}
	response, err := s.grok.Do(request.Context(), http.MethodPost, "/responses", resolvedModel, body, accept)
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		forwardOpenAIError(w, response)
		return
	}
	defer response.Body.Close()
	copyRequestID(w, response.Header)

	if input.Stream {
		copySSE(w, response)
		return
	}

	var output openai.Response
	if err := json.NewDecoder(response.Body).Decode(&output); err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", "invalid upstream response")
		return
	}
	output.Model = requestedModel
	writeJSON(w, http.StatusOK, output)
}

func (s *Server) models(w http.ResponseWriter, request *http.Request) {
	response, err := s.grok.Do(request.Context(), http.MethodGet, "/models", "", nil, "application/json")
	if err != nil {
		writeOpenAIError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	defer response.Body.Close()
	copyRequestID(w, response.Header)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func validateOpenAIInput(raw json.RawMessage) error {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return nil
	}
	var items []openai.InputItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return errors.New("input must be text or an array of response input items")
	}
	for _, item := range items {
		if item.Type != "message" || len(item.Content) == 0 {
			continue
		}
		var content []openai.InputContent
		if err := json.Unmarshal(item.Content, &content); err != nil {
			return errors.New("message content must be an array")
		}
		for _, part := range content {
			if part.Type != "input_text" && part.Type != "output_text" {
				return errors.New("image, audio, and file inputs are not supported")
			}
		}
	}
	return nil
}

func forwardOpenAIError(w http.ResponseWriter, response *http.Response) {
	upstreamError := grok.ReadError(response)
	copyRequestID(w, upstreamError.Header)
	writeOpenAIError(w, upstreamError.Status, openAIErrorType(upstreamError.Status), upstreamMessage(upstreamError))
}

func copySSE(w http.ResponseWriter, response *http.Response) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(response.StatusCode)
	flusher, _ := w.(http.Flusher)
	buffer := make([]byte, 32<<10)
	for {
		count, err := response.Body.Read(buffer)
		if count != 0 {
			if _, writeErr := w.Write(buffer[:count]); writeErr != nil {
				return
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func copyRequestID(w http.ResponseWriter, header http.Header) {
	for _, name := range []string{"x-request-id", "request-id"} {
		if value := header.Get(name); value != "" {
			w.Header().Set(name, value)
		}
	}
}

func upstreamMessage(err *grok.HTTPError) string {
	var response openai.ErrorResponse
	if json.Unmarshal(err.Body, &response) == nil && response.Error.Message != "" {
		return response.Error.Message
	}
	var envelope struct {
		Error json.RawMessage `json:"error"`
	}
	if json.Unmarshal(err.Body, &envelope) == nil {
		var message string
		if json.Unmarshal(envelope.Error, &message) == nil && message != "" {
			return message
		}
	}
	return "upstream request failed"
}
