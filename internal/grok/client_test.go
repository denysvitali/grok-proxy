package grok

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/denysvitali/grok-proxy/internal/auth"
	"github.com/denysvitali/grok-proxy/internal/config"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (fn testRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestDoAddsSubscriptionHeaders(t *testing.T) {
	store := &auth.Store{Path: filepath.Join(t.TempDir(), "auth.json")}
	if err := store.Save(&auth.Token{AccessToken: "secret", ExpiresAt: float64(time.Now().Add(time.Hour).Unix())}); err != nil {
		t.Fatal(err)
	}
	transport := testRoundTripper(func(request *http.Request) (*http.Response, error) {
		checks := map[string]string{
			"Authorization":         "Bearer secret",
			"X-XAI-Token-Auth":      "xai-grok-cli",
			"x-grok-model-override": "grok-4.5",
			"x-grok-client-version": config.ClientVersion,
		}
		for name, expected := range checks {
			if actual := request.Header.Get(name); actual != expected {
				t.Errorf("%s = %q, want %q", name, actual, expected)
			}
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{}`))}, nil
	})
	manager := &auth.Manager{Store: store, HTTPClient: &http.Client{Transport: transport}}
	client := New("https://upstream.example/v1", manager)
	client.HTTP = &http.Client{Transport: transport}
	response, err := client.Do(t.Context(), http.MethodPost, "/responses", "grok-4.5", []byte(`{}`), "application/json")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
}
