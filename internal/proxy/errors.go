package proxy

import (
	"encoding/json"
	"net/http"

	"github.com/denysvitali/grok-proxy/internal/anthropic"
	"github.com/denysvitali/grok-proxy/internal/openai"
)

func writeJSON[T any](w http.ResponseWriter, status int, value T) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeOpenAIError(w http.ResponseWriter, status int, errorType, message string) {
	noteResponseError(w, errorType, message)
	writeJSON(w, status, openai.ErrorResponse{Error: openai.ErrorBody{Type: errorType, Message: message}})
}

func writeAnthropicError(w http.ResponseWriter, status int, errorType, message string) {
	noteResponseError(w, errorType, message)
	writeJSON(w, status, anthropic.ErrorResponse{
		Type:  "error",
		Error: anthropic.ErrorBody{Type: errorType, Message: message},
	})
}

func anthropicErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "authentication_error"
	case http.StatusForbidden:
		return "permission_error"
	case http.StatusNotFound:
		return "not_found_error"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	case 529:
		return "overloaded_error"
	default:
		return "api_error"
	}
}

func openAIErrorType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request_error"
	case http.StatusUnauthorized:
		return "invalid_api_key"
	case http.StatusTooManyRequests:
		return "rate_limit_error"
	default:
		return "api_error"
	}
}
