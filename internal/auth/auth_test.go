package auth

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testRoundTripper func(*http.Request) (*http.Response, error)

func (fn testRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestStoreSavesPrivateFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "auth.json")
	store := &Store{Path: path}
	if err := store.Save(&Token{AccessToken: "secret", ExpiresIn: 3600}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if permission := info.Mode().Perm(); permission != 0600 {
			t.Fatalf("permission = %o, want 600", permission)
		}
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "secret" || loaded.ExpiresAt <= float64(time.Now().Unix()) {
		t.Fatalf("unexpected stored token: %#v", loaded)
	}
}

func TestImportGrokSelectsNewestToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	contents := map[string]map[string]string{
		"old": {"key": "old", "expires_at": "2024-01-01T00:00:00Z"},
		"new": {
			"key":            "new",
			"refresh_token":  "refresh",
			"expires_at":     "2030-01-01T00:00:00Z",
			"oidc_issuer":    "https://issuer.example",
			"oidc_client_id": "client",
		},
	}
	encoded, _ := json.Marshal(contents)
	if err := os.WriteFile(path, encoded, 0600); err != nil {
		t.Fatal(err)
	}
	token, err := ImportGrok(path)
	if err != nil {
		t.Fatal(err)
	}
	if token.AccessToken != "new" || token.RefreshToken != "refresh" || token.Issuer != "https://issuer.example" {
		t.Fatalf("unexpected imported token: %#v", token)
	}
}

func TestManagerRefreshesExpiredTokenOnceForConcurrentRequests(t *testing.T) {
	store := &Store{Path: filepath.Join(t.TempDir(), "auth.json")}
	if err := store.Save(&Token{
		AccessToken:  "expired",
		RefreshToken: "refresh",
		ExpiresAt:    float64(time.Now().Add(-time.Hour).Unix()),
		Issuer:       "https://issuer.example",
		ClientID:     "client",
	}); err != nil {
		t.Fatal(err)
	}
	var refreshes atomic.Int32
	transport := testRoundTripper(func(request *http.Request) (*http.Response, error) {
		body := `{}`
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			body = `{"authorization_endpoint":"https://issuer.example/authorize","device_authorization_endpoint":"https://issuer.example/device","token_endpoint":"https://issuer.example/token"}`
		case "/token":
			refreshes.Add(1)
			body = `{"access_token":"fresh","expires_in":3600}`
		default:
			t.Fatalf("unexpected request path %s", request.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	manager := &Manager{Store: store, HTTPClient: &http.Client{Transport: transport}}
	var group sync.WaitGroup
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			token, err := manager.Usable(t.Context())
			if err != nil {
				t.Errorf("Usable: %v", err)
				return
			}
			if token.AccessToken != "fresh" {
				t.Errorf("access token = %q", token.AccessToken)
			}
		}()
	}
	group.Wait()
	if got := refreshes.Load(); got != 1 {
		t.Fatalf("refresh requests = %d, want 1", got)
	}
}
