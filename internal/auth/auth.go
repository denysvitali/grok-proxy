package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/denysvitali/grok-proxy/internal/config"
)

type Token struct {
	AccessToken  string  `json:"access_token"`
	RefreshToken string  `json:"refresh_token,omitempty"`
	ExpiresIn    float64 `json:"expires_in,omitempty"`
	ExpiresAt    float64 `json:"expires_at,omitempty"`
	Issuer       string  `json:"issuer,omitempty"`
	ClientID     string  `json:"client_id,omitempty"`
	Source       string  `json:"source,omitempty"`
}

type Store struct {
	Path string
	mu   sync.Mutex
}

func (s *Store) Load() (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *Store) load() (*Token, error) {
	b, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.Path, err)
	}
	var token Token
	if err := json.Unmarshal(b, &token); err != nil {
		return nil, fmt.Errorf("decode %s: %w", s.Path, err)
	}
	return &token, nil
}

func (s *Store) Save(token *Token) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save(token)
}

func (s *Store) save(token *Token) error {
	if token.Issuer == "" {
		token.Issuer = config.Issuer
	}
	if token.ClientID == "" {
		token.ClientID = config.ClientID
	}
	if token.ExpiresIn > 0 {
		token.ExpiresAt = float64(time.Now().Unix()) + token.ExpiresIn
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.Path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Chmod(s.Path, 0600)
}

func (s *Store) Clear() error {
	err := os.Remove(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

type Manager struct {
	Store      *Store
	HTTPClient *http.Client
	LegacyPath string
	GrokPath   string
	mu         sync.Mutex
}

func (m *Manager) Usable(ctx context.Context) (*Token, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	token, err := m.Store.Load()
	if err != nil {
		return nil, err
	}
	if token == nil && m.LegacyPath != "" {
		if b, readErr := os.ReadFile(m.LegacyPath); readErr == nil {
			var legacy Token
			if json.Unmarshal(b, &legacy) == nil && legacy.AccessToken != "" {
				token = &legacy
				if err := m.Store.Save(token); err != nil {
					return nil, err
				}
			}
		}
	}
	if token == nil && m.GrokPath != "" {
		if _, statErr := os.Stat(m.GrokPath); statErr == nil {
			token, err = ImportGrok(m.GrokPath)
			if err != nil {
				return nil, err
			}
			if err := m.Store.Save(token); err != nil {
				return nil, err
			}
		}
	}
	if token == nil || token.AccessToken == "" {
		return nil, errors.New("not logged in; run `grok-proxy login`")
	}
	if token.ExpiresAt > 0 && token.ExpiresAt <= float64(time.Now().Add(5*time.Minute).Unix()) {
		if token.RefreshToken == "" {
			return nil, errors.New("token expired and no refresh token is available; run login")
		}
		if err := m.refresh(ctx, token); err != nil {
			return nil, err
		}
		if err := m.Store.Save(token); err != nil {
			return nil, err
		}
	}
	return token, nil
}

func (m *Manager) refresh(ctx context.Context, token *Token) error {
	issuer := token.Issuer
	if issuer == "" {
		issuer = config.Issuer
	}
	d, err := Discover(ctx, m.client(), issuer)
	if err != nil {
		return err
	}
	values := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token.RefreshToken}, "client_id": {first(token.ClientID, config.ClientID)}}
	var fresh Token
	if err := doForm(ctx, m.client(), d.TokenEndpoint, values, &fresh); err != nil {
		return err
	}
	if fresh.AccessToken == "" {
		return errors.New("token response omitted access_token")
	}
	if fresh.RefreshToken == "" {
		fresh.RefreshToken = token.RefreshToken
	}
	fresh.Issuer, fresh.ClientID = issuer, first(token.ClientID, config.ClientID)
	*token = fresh
	return nil
}

func (m *Manager) client() *http.Client {
	if m.HTTPClient != nil {
		return m.HTTPClient
	}
	return &http.Client{Timeout: 30 * time.Second}
}

type Discovery struct {
	AuthorizationEndpoint       string `json:"authorization_endpoint"`
	DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
	TokenEndpoint               string `json:"token_endpoint"`
}

type DeviceAuthorization struct {
	DeviceCode              string  `json:"device_code"`
	UserCode                string  `json:"user_code"`
	VerificationURI         string  `json:"verification_uri"`
	VerificationURIComplete string  `json:"verification_uri_complete"`
	ExpiresIn               float64 `json:"expires_in"`
	Interval                float64 `json:"interval"`
}

func Discover(ctx context.Context, client *http.Client, issuer string) (Discovery, error) {
	var d Discovery
	if err := doJSON(ctx, client, http.MethodGet, strings.TrimRight(issuer, "/")+"/.well-known/openid-configuration", nil, &d); err != nil {
		return d, err
	}
	if d.AuthorizationEndpoint == "" || d.DeviceAuthorizationEndpoint == "" || d.TokenEndpoint == "" {
		return d, errors.New("OIDC discovery is missing required endpoints")
	}
	return d, nil
}

func ImportGrok(path string) (*Token, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	type candidate struct{ Key, RefreshToken, ExpiresAt, Issuer, ClientID string }
	var best candidate
	var bestTime time.Time
	for _, raw := range root {
		var v struct {
			Key          string `json:"key"`
			RefreshToken string `json:"refresh_token"`
			ExpiresAt    string `json:"expires_at"`
			Issuer       string `json:"oidc_issuer"`
			ClientID     string `json:"oidc_client_id"`
		}
		if json.Unmarshal(raw, &v) != nil || v.Key == "" {
			continue
		}
		t, _ := time.Parse(time.RFC3339, v.ExpiresAt)
		if best.Key == "" || t.After(bestTime) {
			best = candidate{v.Key, v.RefreshToken, v.ExpiresAt, v.Issuer, v.ClientID}
			bestTime = t
		}
	}
	if best.Key == "" {
		return nil, fmt.Errorf("no session token found in %s", path)
	}
	expiresAt := float64(0)
	if !bestTime.IsZero() {
		expiresAt = float64(bestTime.Unix())
	}
	return &Token{AccessToken: best.Key, RefreshToken: best.RefreshToken, ExpiresAt: expiresAt, Issuer: first(best.Issuer, config.Issuer), ClientID: first(best.ClientID, config.ClientID), Source: path}, nil
}

func LoginDevice(ctx context.Context, client *http.Client, store *Store, issuer, clientID, scopes string, announce func(string)) error {
	d, err := Discover(ctx, client, issuer)
	if err != nil {
		return err
	}
	var device DeviceAuthorization
	values := url.Values{"client_id": {clientID}, "scope": {scopes}}
	if err := doForm(ctx, client, d.DeviceAuthorizationEndpoint, values, &device); err != nil {
		return err
	}
	if device.DeviceCode == "" {
		return errors.New("device authorization response omitted device_code")
	}
	verification := first(device.VerificationURIComplete, device.VerificationURI)
	announce("Open: " + verification)
	if device.UserCode != "" {
		announce("Code: " + device.UserCode)
	}
	interval := time.Duration(device.Interval * float64(time.Second))
	if interval < time.Second {
		interval = 5 * time.Second
	}
	deadline := time.Now().Add(time.Duration(device.ExpiresIn * float64(time.Second)))
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
		var token Token
		err := doForm(ctx, client, d.TokenEndpoint, url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:device_code"}, "device_code": {device.DeviceCode}, "client_id": {clientID}}, &token)
		if err != nil {
			if strings.Contains(err.Error(), "authorization_pending") {
				continue
			}
			if strings.Contains(err.Error(), "slow_down") {
				interval += 5 * time.Second
				continue
			}
			return err
		}
		token.Issuer, token.ClientID = issuer, clientID
		return store.Save(&token)
	}
	return errors.New("device authorization expired")
}

func LoginBrowser(ctx context.Context, client *http.Client, store *Store, issuer, clientID, scopes string, announce func(string)) error {
	d, err := Discover(ctx, client, issuer)
	if err != nil {
		return err
	}
	verifier := randomURL(48)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])
	state := randomURL(24)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer ln.Close()
	redirect := "http://" + ln.Addr().String() + "/callback"
	result := make(chan url.Values, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		select {
		case result <- r.URL.Query():
		default:
		}
		_, _ = io.WriteString(w, "Login received. You can close this window.")
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go server.Serve(ln)
	q := url.Values{"response_type": {"code"}, "client_id": {clientID}, "redirect_uri": {redirect}, "scope": {scopes}, "state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	authURL := d.AuthorizationEndpoint + "?" + q.Encode()
	announce("Open: " + authURL)
	_ = openBrowser(authURL)
	var values url.Values
	select {
	case values = <-result:
	case <-ctx.Done():
		_ = server.Shutdown(context.Background())
		return ctx.Err()
	case <-time.After(5 * time.Minute):
		_ = server.Shutdown(context.Background())
		return errors.New("OAuth callback timed out")
	}
	_ = server.Shutdown(context.Background())
	if values.Get("state") != state {
		return errors.New("OAuth callback state mismatch")
	}
	if values.Get("error") != "" {
		return fmt.Errorf("authorization failed: %s", values.Get("error"))
	}
	if values.Get("code") == "" {
		return errors.New("OAuth callback omitted authorization code")
	}
	var token Token
	if err := doForm(ctx, client, d.TokenEndpoint, url.Values{"grant_type": {"authorization_code"}, "code": {values.Get("code")}, "redirect_uri": {redirect}, "client_id": {clientID}, "code_verifier": {verifier}}, &token); err != nil {
		return err
	}
	token.Issuer, token.ClientID = issuer, clientID
	return store.Save(&token)
}

func doForm(ctx context.Context, client *http.Client, endpoint string, values url.Values, out any) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return execute(client, req, out)
}

func doJSON(ctx context.Context, client *http.Client, method, endpoint string, body io.Reader, out any) error {
	req, _ := http.NewRequestWithContext(ctx, method, endpoint, body)
	return execute(client, req, out)
}

func execute(client *http.Client, req *http.Request, out any) error {
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, req.URL.Redacted(), strings.TrimSpace(string(b)))
	}
	if len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func randomURL(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func first(v, fallback string) string {
	if v != "" {
		return v
	}
	return fallback
}
func openBrowser(target string) error {
	var command string
	switch runtime.GOOS {
	case "darwin":
		command = "open"
	case "windows":
		command = "rundll32"
	default:
		command = "xdg-open"
	}
	args := []string{target}
	if runtime.GOOS == "windows" {
		args = []string{"url.dll,FileProtocolHandler", target}
	}
	return exec.Command(command, args...).Start()
}
