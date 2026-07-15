package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/denysvitali/grok-proxy/internal/adapter"
	"github.com/denysvitali/grok-proxy/internal/anthropic"
	"github.com/denysvitali/grok-proxy/internal/grok"
	"github.com/denysvitali/grok-proxy/internal/openai"
)

func (s *Server) messages(w http.ResponseWriter, request *http.Request) {
	var input anthropic.MessagesRequest
	if err := s.decodeRequest(w, request, &input); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON request")
		return
	}

	resolvedModel := s.config.ResolveModel(input.Model)
	setProxyRequestMeta(w, resolvedModel, input.Stream)
	upstreamRequest, err := adapter.AnthropicRequest(input, resolvedModel)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", err.Error())
		return
	}
	body, err := json.Marshal(upstreamRequest)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "request could not be encoded")
		return
	}

	accept := "application/json"
	if input.Stream {
		accept = "text/event-stream"
	}
	response, err := s.grok.Do(request.Context(), http.MethodPost, "/responses", resolvedModel, body, accept)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "api_error", err.Error())
		return
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		upstreamError := grok.ReadError(response)
		writeAnthropicError(w, upstreamError.Status, anthropicErrorType(upstreamError.Status), upstreamMessage(upstreamError))
		return
	}
	defer response.Body.Close()
	copyRequestID(w, response.Header)

	if input.Stream {
		writeAnthropicStream(w, response.Body, input.Model)
		return
	}

	var upstreamResponse openai.Response
	if err := json.NewDecoder(response.Body).Decode(&upstreamResponse); err != nil {
		writeAnthropicError(w, http.StatusBadGateway, "api_error", "invalid upstream response")
		return
	}
	writeJSON(w, http.StatusOK, adapter.AnthropicResponse(upstreamResponse, input.Model))
}

type tokenCountResponse struct {
	InputTokens int `json:"input_tokens"`
}

func (s *Server) countTokens(w http.ResponseWriter, request *http.Request) {
	var input anthropic.MessagesRequest
	if err := s.decodeRequest(w, request, &input); err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "invalid JSON request")
		return
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, "invalid_request_error", "request could not be encoded")
		return
	}
	estimate := (len(encoded) + 2) / 3
	if estimate < 1 {
		estimate = 1
	}
	w.Header().Set("Warning", `299 grok-proxy "token count is a conservative estimate"`)
	writeJSON(w, http.StatusOK, tokenCountResponse{InputTokens: estimate})
}
