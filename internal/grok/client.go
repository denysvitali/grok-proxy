package grok

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/denysvitali/grok-proxy/internal/auth"
	"github.com/denysvitali/grok-proxy/internal/config"
)

type Client struct {
	BaseURL string
	Tokens  *auth.Manager
	HTTP    *http.Client
}

func New(baseURL string, tokens *auth.Manager) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), Tokens: tokens, HTTP: &http.Client{Timeout: 0}}
}

func (c *Client) Do(ctx context.Context, method, path, model string, body []byte, accept string) (*http.Response, error) {
	token, err := c.Tokens.Usable(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("X-XAI-Token-Auth", "xai-grok-cli")
	req.Header.Set("x-grok-client-version", config.ClientVersion)
	req.Header.Set("x-grok-client-mode", "cli")
	req.Header.Set("User-Agent", "grok-proxy/"+config.ClientVersion)
	if model != "" {
		req.Header.Set("x-grok-model-override", model)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to Grok failed: %w", err)
	}
	return resp, nil
}

func (c *Client) JSON(ctx context.Context, method, path, model string, request, response any) error {
	var body []byte
	var err error
	if request != nil {
		body, err = json.Marshal(request)
		if err != nil {
			return err
		}
	}
	resp, err := c.Do(ctx, method, path, model, body, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: b}
	}
	if response != nil && len(b) > 0 {
		if err := json.Unmarshal(b, response); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) JSONBytes(ctx context.Context, method, path, model string, body []byte, response any) error {
	resp, err := c.Do(ctx, method, path, model, body, "application/json")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &HTTPError{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: data}
	}
	if response != nil && len(data) > 0 {
		if err := json.Unmarshal(data, response); err != nil {
			return err
		}
	}
	return nil
}

type HTTPError struct {
	Status int
	Header http.Header
	Body   []byte
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("Grok returned HTTP %d: %s", e.Status, strings.TrimSpace(string(e.Body)))
}

func ReadError(resp *http.Response) *HTTPError {
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return &HTTPError{Status: resp.StatusCode, Header: resp.Header.Clone(), Body: b}
}

func DefaultHTTPClient() *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 100, IdleConnTimeout: 90 * time.Second, ResponseHeaderTimeout: 5 * time.Minute}}
}
