package proxy

import (
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"
)

func (s *Server) loginPage(w http.ResponseWriter, _ *http.Request) {
	loginHeaders(w)
	_, _ = fmt.Fprint(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Grok Proxy Login</title></head><body><main><h1>Grok Proxy</h1><p>Sign in with your xAI account. Credentials will be stored by this proxy.</p><form method="post" action="/login"><button type="submit">Sign in with xAI</button></form></main></body></html>`)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil || s.tokens.Store == nil {
		http.Error(w, "login is unavailable", http.StatusServiceUnavailable)
		return
	}
	loginHeaders(w)
	flusher, canFlush := w.(http.Flusher)
	_, _ = fmt.Fprint(w, `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width"><title>Grok Proxy Login</title></head><body><main><h1>Sign in with xAI</h1>`)
	messages := make(chan string, 4)
	result := make(chan error, 1)
	go func() {
		result <- s.tokens.LoginDevice(r.Context(), func(message string) { messages <- message })
	}()
	heartbeat := time.NewTicker(10 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case message := <-messages:
			writeLoginMessage(w, message)
		case err := <-result:
			if err != nil {
				_, _ = fmt.Fprintf(w, `<p>Login failed: %s</p><p><a href="/login">Try again</a></p>`, html.EscapeString(err.Error()))
			} else {
				_, _ = fmt.Fprint(w, `<p>Login successful. You can close this page.</p>`)
			}
			_, _ = fmt.Fprint(w, `</main></body></html>`)
			return
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, "<!-- waiting for device authorization -->")
		case <-r.Context().Done():
			return
		}
		if canFlush {
			flusher.Flush()
		}
	}
}

func writeLoginMessage(w http.ResponseWriter, message string) {
	if target, ok := strings.CutPrefix(message, "Open: "); ok {
		escaped := html.EscapeString(target)
		_, _ = fmt.Fprintf(w, `<p><a href="%s" target="_blank" rel="noreferrer">Continue to xAI</a></p>`, escaped)
	} else {
		_, _ = fmt.Fprintf(w, "<p>%s</p>", html.EscapeString(message))
	}
}

func loginHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
