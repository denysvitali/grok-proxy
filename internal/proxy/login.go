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
	writeHTMLHead(w, "Grok Proxy Login")
	_, _ = fmt.Fprint(w, `
<main>
  <div class="login-wrap">
    <section class="card login-card">
      <div class="brand-mark" style="margin-bottom:18px">
        <div class="logo">G</div>
        <div class="eyebrow">Secure device login</div>
      </div>
      <h1 style="font-size:clamp(1.9rem,4vw,2.5rem);margin-bottom:10px">Grok Proxy</h1>
      <p class="lede">Sign in with your xAI account. Credentials are stored by this proxy for local Claude Code and Codex traffic.</p>
      <div class="login-steps">
        <div class="step">
          <div class="step-index">1</div>
          <div>
            <strong>Start device authorization</strong>
            <div class="subline" style="margin:4px 0 0">This proxy opens an xAI device login flow.</div>
          </div>
        </div>
        <div class="step">
          <div class="step-index">2</div>
          <div>
            <strong>Approve in the browser</strong>
            <div class="subline" style="margin:4px 0 0">Confirm the request with your xAI account.</div>
          </div>
        </div>
        <div class="step">
          <div class="step-index">3</div>
          <div>
            <strong>Return to the dashboard</strong>
            <div class="subline" style="margin:4px 0 0">Usage and account details appear once tokens are saved.</div>
          </div>
        </div>
      </div>
      <form method="post" action="/login">
        <button class="btn-primary primary" type="submit">Sign in with xAI</button>
      </form>
      <div class="inline-actions">
        <a class="btn btn-ghost" href="/">Back to dashboard</a>
      </div>
    </section>
  </div>
</main>`)
	writeHTMLFoot(w)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if s.tokens == nil || s.tokens.Store == nil {
		http.Error(w, "login is unavailable", http.StatusServiceUnavailable)
		return
	}
	loginHeaders(w)
	flusher, canFlush := w.(http.Flusher)
	writeHTMLHead(w, "Grok Proxy Login")
	_, _ = fmt.Fprint(w, `
<main>
  <div class="login-wrap">
    <section class="card login-card">
      <div class="brand-mark" style="margin-bottom:18px">
        <div class="logo">G</div>
        <div class="eyebrow">Waiting for authorization</div>
      </div>
      <h1 style="font-size:clamp(1.9rem,4vw,2.5rem);margin-bottom:10px">Sign in with xAI</h1>
      <p class="lede">Keep this page open while device authorization completes.</p>
      <div class="stream-log">`)
	if canFlush {
		flusher.Flush()
	}

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
				_, _ = fmt.Fprintf(w, `</div><div class="error-box">Login failed: %s</div><div class="inline-actions"><a class="btn" href="/login">Try again</a><a class="btn btn-ghost" href="/">Dashboard</a></div></section></div></main>`, html.EscapeString(err.Error()))
			} else {
				_, _ = fmt.Fprint(w, `</div><div class="success-box">Login successful. Tokens were stored for this proxy.</div><div class="inline-actions"><a class="btn btn-primary" href="/">Open dashboard</a></div></section></div></main>`)
			}
			writeHTMLFoot(w)
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
		_, _ = fmt.Fprintf(w, `<div class="stream-item"><a href="%s" target="_blank" rel="noreferrer">Continue to xAI</a></div>`, escaped)
		return
	}
	_, _ = fmt.Fprintf(w, `<div class="stream-item">%s</div>`, html.EscapeString(message))
}
