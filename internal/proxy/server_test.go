package proxy

import (
	"bytes"
	"context"
	"encoding/base64"
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

func TestReadyzAndMetricsDoNotRequireAuthentication(t *testing.T) {
	server := New(config.Config{Server: config.ServerConfig{APIKey: "secret"}}, nil, nil, testLogger())
	for _, path := range []string{"/readyz", "/metrics"} {
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, path, nil)
		recorder := httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d body = %s", path, recorder.Code, recorder.Body.String())
		}
	}
	metricsBody := httptest.NewRecorder()
	server.Handler().ServeHTTP(metricsBody, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsBody.Body.String(), "grok_proxy_http_requests_total") {
		t.Fatalf("metrics missing request counter: %s", metricsBody.Body.String())
	}
}

func TestRequestLogsCaptureClientErrorsAndSkipProbeNoise(t *testing.T) {
	logger := logrus.New()
	var logs bytes.Buffer
	logger.SetOutput(&logs)
	logger.SetLevel(logrus.InfoLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})

	store := &auth.Store{Path: filepath.Join(t.TempDir(), "auth.json")}
	if err := store.Save(&auth.Token{AccessToken: "subscription-token", ExpiresAt: float64(time.Now().Add(time.Hour).Unix())}); err != nil {
		t.Fatal(err)
	}
	manager := &auth.Manager{Store: store, HTTPClient: &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("unsupported request reached upstream")
		return nil, nil
	})}}
	client := grok.New("https://upstream.example/v1", manager)
	client.HTTP = &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		t.Fatal("unsupported request reached upstream")
		return nil, nil
	})}
	handler := New(config.Config{
		Server: config.ServerConfig{Listen: "127.0.0.1:8080", MaxBodyBytes: 1 << 20},
		Proxy:  config.ProxyConfig{DefaultModel: "grok-4.5", ModelMap: map[string]string{}},
	}, client, manager, logger).Handler()

	probe := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	probeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(probeRecorder, probe)
	if probeRecorder.Code != http.StatusOK {
		t.Fatalf("health status = %d", probeRecorder.Code)
	}
	if logs.Len() != 0 {
		t.Fatalf("expected quiet healthz logs, got %s", logs.String())
	}

	body := `{"model":"grok-4.5","input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"https://example.test/image.png"}]}]}`
	request := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	request.Header.Set("User-Agent", "test-agent/1.0")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", recorder.Code, recorder.Body.String())
	}
	logLine := logs.String()
	for _, expected := range []string{`"level":"warning"`, `"error_type":"invalid_request_error"`, `"path":"/v1/responses"`, `"model":"grok-4.5"`, `"protocol":"openai"`} {
		if !strings.Contains(logLine, expected) {
			t.Fatalf("log missing %s: %s", expected, logLine)
		}
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

func TestDashboardPromptsForLoginWithoutCredentials(t *testing.T) {
	manager := &auth.Manager{Store: &auth.Store{Path: filepath.Join(t.TempDir(), "auth.json")}}
	server := New(config.Config{}, nil, manager, testLogger())
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Host = "proxy.example:8443"
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "Sign in to view your usage") {
		t.Fatalf("unexpected dashboard: %s", recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	page := recorder.Body.String()
	for _, expected := range []string{"Claude Code", `value="proxy.example:8443"`, "ANTHROPIC_BASE_URL=", "Copy command"} {
		if !strings.Contains(page, expected) {
			t.Errorf("dashboard missing %q: %s", expected, page)
		}
	}
	if strings.Contains(page, "\\n+ANTHROPIC_AUTH_TOKEN") {
		t.Fatalf("Claude Code command contains an unexpected plus sign: %s", page)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); !strings.Contains(got, "script-src 'unsafe-inline'") {
		t.Errorf("Content-Security-Policy = %q", got)
	}
}

func TestDashboardDisplaysAccountUsageAndProxyStatus(t *testing.T) {
	handler := newTestServerWithConfig(t, config.Config{
		BaseURL: "https://upstream.example/v1",
		Server:  config.ServerConfig{Listen: "127.0.0.1:8080", MaxBodyBytes: 1 << 20},
		Proxy:   config.ProxyConfig{DefaultModel: "grok-4.5", ModelMap: map[string]string{}},
	}, func(request *http.Request) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); got != "Bearer subscription-token" {
			t.Errorf("Authorization = %q", got)
		}
		if request.URL.Host != "upstream.example" {
			t.Errorf("dashboard host = %q", request.URL.Host)
		}
		if got := request.Header.Get("x-grok-client-mode"); got != "interactive" {
			t.Errorf("client mode = %q", got)
		}
		if got := request.Header.Get("x-grok-client-version"); got != config.ClientVersion {
			t.Errorf("client version = %q", got)
		}
		if got := request.Header.Get("X-XAI-Token-Auth"); got != "xai-grok-cli" {
			t.Errorf("token-auth header = %q", got)
		}
		switch request.URL.Path {
		case "/v1/user":
			if request.URL.Query().Get("include") != "subscription" {
				t.Errorf("account URL = %s", request.URL)
			}
			if got := request.Header.Get("x-userid"); got != "" {
				t.Errorf("account user ID = %q", got)
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"userId": "user-123", "firstName": "Ada", "lastName": "Lovelace",
				"teamName": "Analytical Engines", "subscriptionTier": "SuperGrok",
				"hasGrokCodeAccess": true, "codingDataRetentionOptOut": true,
			}), nil
		case "/v1/billing":
			if request.URL.Query().Get("format") != "credits" {
				t.Errorf("billing URL = %s", request.URL)
			}
			if got := request.Header.Get("x-userid"); got != "user-123" {
				t.Errorf("billing user ID = %q", got)
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"config": map[string]any{
					"creditUsagePercent": 42.5, "monthlyLimit": map[string]any{"val": 1000},
					"used": map[string]any{"val": 425}, "onDemandUsed": map[string]any{"val": -12},
					"prepaidBalance": map[string]any{"val": 25},
					"currentPeriod":  map[string]any{"type": "USAGE_PERIOD_TYPE_WEEKLY", "start": "2026-07-13", "end": "2026-07-20"},
				},
				"onDemandEnabled": true, "subscriptionTier": "SuperGrok",
			}), nil
		default:
			t.Fatalf("unexpected dashboard request: %s", request.URL)
			return nil, nil
		}
	})

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	page := recorder.Body.String()
	for _, expected := range []string{"Ada Lovelace", "SuperGrok", "42.5%", "$4.25", "$0.12", "WEEKLY", "grok-4.5", "Opted out"} {
		if !strings.Contains(page, expected) {
			t.Errorf("dashboard missing %q: %s", expected, page)
		}
	}
	if strings.Contains(page, "subscription-token") {
		t.Fatal("dashboard exposed the access token")
	}
}

func TestDashboardFallsBackToOAuthClaimsWhenAccountEnrichmentFails(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"userId":"user-456","first_name":"Grace","last_name":"Hopper","email":"grace@example.com"}`))
	accessToken := "header." + payload + ".signature"
	store := &auth.Store{Path: filepath.Join(t.TempDir(), "auth.json")}
	if err := store.Save(&auth.Token{AccessToken: accessToken, ExpiresAt: float64(time.Now().Add(time.Hour).Unix())}); err != nil {
		t.Fatal(err)
	}
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/v1/user" {
			return jsonResponse(http.StatusForbidden, map[string]any{"error": "forbidden"}), nil
		}
		return jsonResponse(http.StatusOK, map[string]any{"config": map[string]any{"creditUsagePercent": 10}}), nil
	})
	manager := &auth.Manager{Store: store, HTTPClient: &http.Client{Transport: transport}}
	server := New(config.Config{}, nil, manager, testLogger())
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	page := recorder.Body.String()
	for _, expected := range []string{"Grace Hopper", "grace@example.com", "Some account details could not be loaded."} {
		if !strings.Contains(page, expected) {
			t.Errorf("dashboard missing %q: %s", expected, page)
		}
	}
	if strings.Contains(page, accessToken) {
		t.Fatal("dashboard exposed the access token")
	}
}

func TestUsageViewFallsBackToLegacyLimitAndUsed(t *testing.T) {
	view := usageView(grok.Billing{
		Available:    true,
		MonthlyLimit: grok.Number{Value: 1000, Valid: true},
		Used:         grok.Number{Value: 425, Valid: true},
	})
	if !view.HasPercent || view.Percent != "42.5%" || view.PercentValue != "42.50" {
		t.Fatalf("usage percentage = %#v", view)
	}
}

func TestUsageViewShowsZeroWhenPercentageAndLegacyLimitAreAbsent(t *testing.T) {
	view := usageView(grok.Billing{Available: true})
	if !view.HasPercent || view.Percent != "0.0%" || view.PercentValue != "0.00" {
		t.Fatalf("usage percentage = %#v", view)
	}
}
