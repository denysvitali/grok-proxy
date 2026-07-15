package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denysvitali/grok-proxy/internal/anthropic"
	"github.com/denysvitali/grok-proxy/internal/auth"
	"github.com/denysvitali/grok-proxy/internal/config"
	"github.com/denysvitali/grok-proxy/internal/grok"
	"github.com/denysvitali/grok-proxy/internal/openai"
	"github.com/sirupsen/logrus"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestResponsesHandlerMapsModelAndAddsSubscriptionHeaders(t *testing.T) {
	var upstreamRequest openai.ResponsesRequest
	handler := newTestServer(t, func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("X-XAI-Token-Auth"); got != "xai-grok-cli" {
			t.Errorf("X-XAI-Token-Auth = %q", got)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer subscription-token" {
			t.Errorf("Authorization = %q", got)
		}
		if err := json.NewDecoder(request.Body).Decode(&upstreamRequest); err != nil {
			t.Fatal(err)
		}
		response := openai.Response{ID: "resp_1", Model: "grok-4.5", Output: []openai.OutputItem{}}
		return jsonResponse(http.StatusOK, response), nil
	})

	body := `{"model":"gpt-codex","input":"hello","stream":false}`
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if upstreamRequest.Model != "grok-4.5" {
		t.Fatalf("upstream model = %q", upstreamRequest.Model)
	}
	var downstream openai.Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &downstream); err != nil {
		t.Fatal(err)
	}
	if downstream.Model != "gpt-codex" {
		t.Fatalf("downstream model = %q", downstream.Model)
	}
}

func TestAnthropicHandlerTranslatesNonStreamingResponse(t *testing.T) {
	handler := newTestServer(t, func(request *http.Request) (*http.Response, error) {
		var upstream openai.ResponsesRequest
		if err := json.NewDecoder(request.Body).Decode(&upstream); err != nil {
			t.Fatal(err)
		}
		if upstream.Model != "grok-4.5" || upstream.Instructions != "Help" {
			t.Fatalf("unexpected upstream request: %#v", upstream)
		}
		response := openai.Response{
			ID: "resp_1",
			Output: []openai.OutputItem{{
				Type:    "message",
				Content: []openai.OutputContent{{Type: "output_text", Text: "done"}},
			}},
			Usage: openai.Usage{InputTokens: 4, OutputTokens: 1},
		}
		return jsonResponse(http.StatusOK, response), nil
	})

	body := `{"model":"claude-sonnet","system":"Help","messages":[{"role":"user","content":"hello"}],"max_tokens":100}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response anthropic.MessagesResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Model != "claude-sonnet" || response.Content[0].Text != "done" || response.Usage.InputTokens != 4 {
		t.Fatalf("unexpected response: %#v", response)
	}
}

func TestAnthropicStreamingEventOrder(t *testing.T) {
	handler := newTestServer(t, func(_ *http.Request) (*http.Response, error) {
		stream := strings.Join([]string{
			`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","role":"assistant"}}`,
			"",
			`data: {"type":"response.output_text.delta","output_index":0,"delta":"hello"}`,
			"",
			`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message"}}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":1}}}`,
			"",
		}, "\n")
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(stream))}, nil
	})

	body := `{"model":"claude-sonnet","messages":[{"role":"user","content":"hello"}],"max_tokens":100,"stream":true}`
	request := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	stream := recorder.Body.String()
	events := []string{"event: message_start", "event: content_block_start", "event: content_block_delta", "event: content_block_stop", "event: message_delta", "event: message_stop"}
	position := -1
	for _, event := range events {
		next := strings.Index(stream[position+1:], event)
		if next < 0 {
			t.Fatalf("missing %q in stream:\n%s", event, stream)
		}
		position += next + 1
	}
}

func TestLiveStyleUpstreamErrorIsNormalizedForBothProtocols(t *testing.T) {
	handler := newTestServer(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusPaymentRequired,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"error":"Grok Build usage balance exhausted"}`)),
		}, nil
	})

	tests := []struct {
		path string
		body string
	}{
		{path: "/v1/responses", body: `{"model":"gpt-codex","input":"hello"}`},
		{path: "/v1/messages", body: `{"model":"claude-sonnet","max_tokens":32,"messages":[{"role":"user","content":"hello"}]}`},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusPaymentRequired {
			t.Errorf("%s status = %d", test.path, recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "Grok Build usage balance exhausted") {
			t.Errorf("%s lost upstream message: %s", test.path, recorder.Body.String())
		}
	}
}

func TestAPIKeyIsRequiredWhenConfigured(t *testing.T) {
	handler := newTestServerWithConfig(t, config.Config{
		Server: config.ServerConfig{Listen: "127.0.0.1:8080", APIKey: "proxy-secret", MaxBodyBytes: 1 << 20},
		Proxy:  config.ProxyConfig{DefaultModel: "grok-4.5", ModelMap: map[string]string{}},
	}, func(_ *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, openai.Response{}), nil
	})

	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"grok-4.5","input":"hi"}`))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestResponsesRejectImageInput(t *testing.T) {
	handler := newTestServer(t, func(_ *http.Request) (*http.Response, error) {
		t.Fatal("unsupported request reached upstream")
		return nil, nil
	})
	body := `{"model":"grok-4.5","input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.test/image.png"}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestNonLoopbackWithoutKeyIsRejected(t *testing.T) {
	server := New(config.Config{Server: config.ServerConfig{Listen: "0.0.0.0:8080"}}, nil, nil, testLogger())
	if err := server.ValidateListenAddress(); err == nil {
		t.Fatal("expected unsafe listener to be rejected")
	}
}

func newTestServer(t *testing.T, transport roundTripFunc) http.Handler {
	t.Helper()
	return newTestServerWithConfig(t, config.Config{
		Server: config.ServerConfig{Listen: "127.0.0.1:8080", MaxBodyBytes: 1 << 20},
		Proxy:  config.ProxyConfig{DefaultModel: "grok-4.5", ModelMap: map[string]string{}},
	}, transport)
}

func newTestServerWithConfig(t *testing.T, cfg config.Config, transport roundTripFunc) http.Handler {
	t.Helper()
	store := &auth.Store{Path: filepath.Join(t.TempDir(), "auth.json")}
	if err := store.Save(&auth.Token{AccessToken: "subscription-token", ExpiresAt: float64(time.Now().Add(time.Hour).Unix())}); err != nil {
		t.Fatal(err)
	}
	manager := &auth.Manager{Store: store, HTTPClient: &http.Client{Transport: transport}}
	client := grok.New("https://upstream.example/v1", manager)
	client.HTTP = &http.Client{Transport: transport}
	return New(cfg, client, manager, testLogger()).Handler()
}

func jsonResponse[T any](status int, value T) *http.Response {
	encoded, _ := json.Marshal(value)
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(encoded))}
}

func testLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return logger
}

func TestHealthDoesNotRequireAuthentication(t *testing.T) {
	server := New(config.Config{Server: config.ServerConfig{APIKey: "secret"}}, nil, nil, testLogger())
	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if recorder.Header().Get("x-request-id") == "" {
		t.Fatal("health response is missing x-request-id")
	}
}

func TestLoginPageDoesNotRequireAPIAuthentication(t *testing.T) {
	server := New(config.Config{Server: config.ServerConfig{APIKey: "secret"}}, nil, nil, testLogger())
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Sign in with xAI") {
		t.Fatalf("unexpected login page: %s", recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}
