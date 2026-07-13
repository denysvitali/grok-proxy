package proxy

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

func (s *Server) authenticate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		expected := s.config.Server.APIKey
		if expected == "" {
			next(w, request)
			return
		}

		provided := request.Header.Get("x-api-key")
		if authorization := request.Header.Get("Authorization"); strings.HasPrefix(authorization, "Bearer ") {
			provided = strings.TrimPrefix(authorization, "Bearer ")
		}
		if len(provided) != len(expected) || subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			if strings.Contains(request.URL.Path, "messages") {
				writeAnthropicError(w, http.StatusUnauthorized, "authentication_error", "invalid API key")
			} else {
				writeOpenAIError(w, http.StatusUnauthorized, "invalid_api_key", "invalid API key")
			}
			return
		}
		next(w, request)
	}
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		started := time.Now()
		response := &loggingResponseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(response, request)
		s.log.WithFields(logrus.Fields{
			"method":      request.Method,
			"path":        request.URL.Path,
			"status":      response.status,
			"request_id":  response.Header().Get("x-request-id"),
			"duration_ms": time.Since(started).Milliseconds(),
		}).Info("request")
	})
}

func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requestID := request.Header.Get("x-request-id")
		if requestID == "" || len(requestID) > 128 {
			requestID = newRequestID()
		}
		w.Header().Set("x-request-id", requestID)
		next.ServeHTTP(w, request)
	})
}

func (s *Server) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.WithField("panic", fmt.Sprint(recovered)).Error("request panic")
				writeOpenAIError(w, http.StatusInternalServerError, "api_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, request)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.wroteHeader {
		return
	}
	w.status = status
	w.wroteHeader = true
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(data []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

func (w *loggingResponseWriter) Flush() {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func newRequestID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("local-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}
