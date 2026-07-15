package proxy

import (
	"fmt"
	"net/http"
)

func setDashboardHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func loginHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

// Mobile-first design system. Base styles target ~360px phones; min-width
// breakpoints add multi-column layout for tablet and desktop.
const uiStyles = `
:root {
  color-scheme: dark;
  --bg: #07080c;
  --bg-elevated: rgba(18, 20, 28, 0.92);
  --bg-soft: rgba(255, 255, 255, 0.03);
  --border: rgba(255, 255, 255, 0.08);
  --border-strong: rgba(255, 255, 255, 0.14);
  --text: #f4f6fb;
  --muted: #9aa3b5;
  --faint: #6d7588;
  --accent: #8b7cf6;
  --accent-soft: rgba(139, 124, 246, 0.16);
  --ok: #34d399;
  --ok-soft: rgba(52, 211, 153, 0.14);
  --warn: #fbbf24;
  --warn-soft: rgba(251, 191, 36, 0.12);
  --danger: #fb7185;
  --danger-soft: rgba(251, 113, 133, 0.12);
  --shadow: 0 16px 40px rgba(0, 0, 0, 0.35);
  --radius: 18px;
  --radius-sm: 14px;
  --space: 16px;
  --pad-x: 16px;
  --font: "Segoe UI", Inter, ui-sans-serif, system-ui, -apple-system, sans-serif;
  --mono: ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace;
  --touch: 44px;
}
*, *::before, *::after { box-sizing: border-box; }
html {
  -webkit-text-size-adjust: 100%;
  text-size-adjust: 100%;
  scroll-behavior: smooth;
}
body {
  margin: 0;
  min-height: 100vh;
  min-height: 100dvh;
  font-family: var(--font);
  color: var(--text);
  background:
    radial-gradient(680px 360px at 10% -8%, rgba(139, 124, 246, 0.2), transparent 55%),
    radial-gradient(520px 280px at 100% 0%, rgba(70, 214, 192, 0.12), transparent 50%),
    linear-gradient(180deg, #0b0d13 0%, var(--bg) 48%, #06070b 100%);
  overflow-x: hidden;
}
body::before {
  content: "";
  position: fixed;
  inset: 0;
  pointer-events: none;
  background-image:
    linear-gradient(rgba(255,255,255,0.02) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255,255,255,0.02) 1px, transparent 1px);
  background-size: 40px 40px;
  mask-image: radial-gradient(circle at center, black, transparent 80%);
  opacity: 0.45;
}
img, svg, video { max-width: 100%; height: auto; }
a { color: inherit; }
button, input, textarea { max-width: 100%; }
main {
  position: relative;
  width: 100%;
  max-width: 1120px;
  margin: 0 auto;
  padding: 20px var(--pad-x) 48px;
}
.shell {
  display: grid;
  gap: var(--space);
  min-width: 0;
}
.topbar {
  display: grid;
  gap: 16px;
  min-width: 0;
}
.brand {
  display: grid;
  gap: 10px;
  min-width: 0;
}
.brand-mark {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
}
.logo {
  flex: 0 0 auto;
  width: 36px;
  height: 36px;
  border-radius: 11px;
  display: grid;
  place-items: center;
  background: linear-gradient(145deg, rgba(139,124,246,0.95), rgba(70,214,192,0.85));
  box-shadow: 0 8px 24px rgba(139, 124, 246, 0.28);
  font-weight: 800;
  letter-spacing: -0.04em;
  color: #0b0d13;
}
.eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  color: var(--ok);
  font-size: 0.8rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}
.eyebrow::before {
  content: "";
  flex: 0 0 auto;
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--ok);
  box-shadow: 0 0 0 5px var(--ok-soft), 0 0 18px rgba(52, 211, 153, 0.7);
  animation: pulse 2.4s ease-in-out infinite;
}
h1, h2, h3, p { margin: 0; overflow-wrap: anywhere; }
h1 {
  font-size: clamp(1.75rem, 8vw, 2.25rem);
  line-height: 1.08;
  letter-spacing: -0.045em;
  font-weight: 760;
}
.lede {
  max-width: 42rem;
  color: var(--muted);
  font-size: 0.95rem;
  line-height: 1.55;
  overflow-wrap: anywhere;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-start;
}
.btn, button, .chip {
  appearance: none;
  border: 1px solid var(--border-strong);
  background: rgba(255,255,255,0.04);
  color: var(--text);
  border-radius: 999px;
  min-height: var(--touch);
  padding: 10px 14px;
  font: inherit;
  font-size: 0.92rem;
  font-weight: 600;
  text-decoration: none;
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  transition: background 0.15s ease, border-color 0.15s ease, transform 0.15s ease, box-shadow 0.15s ease;
  -webkit-tap-highlight-color: transparent;
}
.btn:hover, button:hover {
  background: rgba(255,255,255,0.08);
  border-color: rgba(255,255,255,0.22);
}
.btn:active, button:active { transform: scale(0.98); }
.btn-primary, button.primary {
  background: linear-gradient(135deg, #f8fafc, #dbe4ff);
  color: #0b1020;
  border-color: transparent;
  box-shadow: 0 8px 22px rgba(219, 228, 255, 0.16);
  width: 100%;
}
.btn-primary:hover, button.primary:hover {
  background: linear-gradient(135deg, #ffffff, #e8eeff);
}
.btn-ghost { background: transparent; }
.grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: var(--space);
  min-width: 0;
}
.card {
  position: relative;
  overflow: hidden;
  min-width: 0;
  width: 100%;
  background:
    linear-gradient(180deg, rgba(255,255,255,0.03), transparent 34%),
    var(--bg-elevated);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 16px;
  box-shadow: var(--shadow);
  backdrop-filter: blur(14px);
}
.card::after {
  content: "";
  position: absolute;
  inset: 0 auto auto 0;
  width: 140px;
  height: 140px;
  background: radial-gradient(circle, var(--accent-soft), transparent 68%);
  opacity: 0.65;
  pointer-events: none;
}
.card > * { position: relative; z-index: 1; min-width: 0; }
.card-wide { width: 100%; }
.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 10px;
  margin-bottom: 14px;
  min-width: 0;
}
.section-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--muted);
  font-size: 0.72rem;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.section-label .dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: var(--accent);
  box-shadow: 0 0 0 4px var(--accent-soft);
  flex: 0 0 auto;
}
.identity {
  font-size: clamp(1.25rem, 6vw, 1.7rem);
  font-weight: 720;
  letter-spacing: -0.03em;
  margin-bottom: 6px;
  overflow-wrap: anywhere;
}
.subline {
  color: var(--muted);
  margin-bottom: 14px;
  line-height: 1.5;
  font-size: 0.92rem;
  overflow-wrap: anywhere;
}
.meta-list {
  display: grid;
  gap: 0;
  margin: 0;
  min-width: 0;
}
.meta-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 4px;
  padding: 12px 0;
  border-top: 1px solid rgba(255,255,255,0.06);
  min-width: 0;
}
.meta-row:first-child { border-top: 0; padding-top: 2px; }
.meta-row dt {
  color: var(--muted);
  font-size: 0.84rem;
}
.meta-row dd {
  margin: 0;
  overflow-wrap: anywhere;
  word-break: break-word;
  font-weight: 560;
  font-size: 0.95rem;
}
.usage-hero {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 14px;
  align-items: center;
  margin-bottom: 14px;
  min-width: 0;
}
.ring {
  --value: 0;
  width: 104px;
  height: 104px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  justify-self: start;
  background:
    radial-gradient(circle at center, rgba(10,12,18,0.96) 58%, transparent 59%),
    conic-gradient(from 210deg, var(--accent) calc(var(--value) * 1%), rgba(255,255,255,0.08) 0);
  box-shadow: inset 0 0 0 1px rgba(255,255,255,0.05);
}
.ring-inner {
  width: 74px;
  height: 74px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  text-align: center;
  background: rgba(8, 10, 16, 0.96);
  border: 1px solid rgba(255,255,255,0.06);
}
.usage-value {
  font-size: 1.2rem;
  font-weight: 760;
  letter-spacing: -0.04em;
}
.usage-caption {
  color: var(--faint);
  font-size: 0.68rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.usage-copy { min-width: 0; }
.usage-copy h3 {
  font-size: 1.05rem;
  margin-bottom: 6px;
}
.usage-copy p {
  color: var(--muted);
  line-height: 1.5;
  font-size: 0.92rem;
}
.pill-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 12px;
}
.pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  max-width: 100%;
  padding: 7px 11px;
  border-radius: 999px;
  background: var(--bg-soft);
  border: 1px solid var(--border);
  color: var(--muted);
  font-size: 0.8rem;
  font-weight: 600;
  overflow-wrap: anywhere;
}
.pill strong { color: var(--text); font-weight: 700; }
.notice {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 12px 13px;
  border-radius: 12px;
  border: 1px solid rgba(251, 191, 36, 0.28);
  background: var(--warn-soft);
  color: #fde68a;
  margin-bottom: 14px;
  line-height: 1.45;
  overflow-wrap: anywhere;
  font-size: 0.92rem;
}
.notice-error {
  border-color: rgba(251, 113, 133, 0.28);
  background: var(--danger-soft);
  color: #fecdd3;
}
.empty { text-align: left; }
.empty-panel {
  display: grid;
  gap: 14px;
  padding: 18px;
  border-radius: calc(var(--radius) - 2px);
  border: 1px dashed rgba(255,255,255,0.12);
  background:
    linear-gradient(180deg, rgba(139,124,246,0.08), transparent 55%),
    rgba(255,255,255,0.02);
  min-width: 0;
}
.empty-kicker {
  color: var(--accent);
  font-size: 0.74rem;
  font-weight: 700;
  letter-spacing: 0.1em;
  text-transform: uppercase;
}
.setup {
  display: grid;
  gap: 14px;
  min-width: 0;
}
.setup-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 12px;
  min-width: 0;
}
.setup-block {
  display: grid;
  gap: 8px;
  padding: 14px;
  border-radius: var(--radius-sm);
  background: rgba(255,255,255,0.025);
  border: 1px solid rgba(255,255,255,0.06);
  min-width: 0;
}
.setup-block label {
  color: var(--muted);
  font-size: 0.82rem;
  font-weight: 600;
}
.setup input,
.setup textarea {
  width: 100%;
  min-width: 0;
  color: var(--text);
  background: rgba(7, 8, 12, 0.88);
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 12px;
  padding: 12px;
  font: 0.84rem/1.45 var(--mono);
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.setup input:focus,
.setup textarea:focus {
  border-color: rgba(139, 124, 246, 0.7);
  box-shadow: 0 0 0 4px rgba(139, 124, 246, 0.14);
}
.setup textarea {
  min-height: 128px;
  resize: vertical;
  field-sizing: content;
}
.setup-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}
.setup-actions .btn,
.setup-actions button {
  width: 100%;
}
.copy-status {
  color: var(--muted);
  font-size: 0.85rem;
  min-height: 1.2em;
}
.stat-grid {
  display: grid;
  grid-template-columns: minmax(0, 1fr);
  gap: 10px;
  min-width: 0;
}
.stat {
  padding: 12px 13px;
  border-radius: 14px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.06);
  min-width: 0;
}
.stat-label {
  color: var(--muted);
  font-size: 0.78rem;
  margin-bottom: 6px;
}
.stat-value {
  font-size: 0.98rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.footer {
  color: var(--faint);
  font-size: 0.84rem;
  line-height: 1.5;
  padding: 2px 2px 0;
  overflow-wrap: anywhere;
}
.login-wrap {
  min-height: calc(100dvh - 40px);
  display: grid;
  align-content: start;
  padding: 8px 0 24px;
}
.login-card {
  width: 100%;
  max-width: 520px;
  margin: 0 auto;
  padding: 18px;
}
.login-title {
  font-size: clamp(1.7rem, 7vw, 2.3rem);
  margin-bottom: 10px;
}
.login-steps {
  display: grid;
  gap: 10px;
  margin: 18px 0;
}
.step {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 12px;
  align-items: start;
  padding: 12px;
  border-radius: 14px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.06);
  min-width: 0;
}
.step-index {
  width: 28px;
  height: 28px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background: var(--accent-soft);
  color: #d8d2ff;
  font-size: 0.82rem;
  font-weight: 700;
  flex: 0 0 auto;
}
.stream-log {
  display: grid;
  gap: 10px;
  margin: 16px 0 8px;
  min-width: 0;
}
.stream-item {
  padding: 12px 13px;
  border-radius: 12px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.06);
  color: var(--muted);
  line-height: 1.45;
  overflow-wrap: anywhere;
  word-break: break-word;
}
.stream-item a {
  color: #c4b5fd;
  font-weight: 700;
  text-decoration: none;
  overflow-wrap: anywhere;
}
.stream-item a:hover { text-decoration: underline; }
.success-box, .error-box {
  margin-top: 14px;
  padding: 12px 13px;
  border-radius: 12px;
  line-height: 1.45;
  overflow-wrap: anywhere;
}
.success-box {
  background: var(--ok-soft);
  border: 1px solid rgba(52, 211, 153, 0.28);
  color: #a7f3d0;
}
.error-box {
  background: var(--danger-soft);
  border: 1px solid rgba(251, 113, 133, 0.28);
  color: #fecdd3;
}
.inline-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 14px;
}
.inline-actions .btn,
.inline-actions a.btn {
  flex: 1 1 100%;
}
form { min-width: 0; }
code {
  font-family: var(--mono);
  font-size: 0.88em;
  padding: 0.1em 0.35em;
  border-radius: 7px;
  background: rgba(255,255,255,0.06);
  border: 1px solid rgba(255,255,255,0.06);
  overflow-wrap: anywhere;
  word-break: break-word;
}
@keyframes pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.12); opacity: 0.75; }
}

/* Tablet+ */
@media (min-width: 640px) {
  :root {
    --pad-x: 24px;
    --space: 18px;
    --radius: 20px;
  }
  main { padding-top: 28px; padding-bottom: 56px; }
  h1 { font-size: clamp(2rem, 5vw, 2.8rem); }
  .lede { font-size: 1rem; }
  .card { padding: 20px; }
  .btn-primary, button.primary { width: auto; }
  .setup-actions .btn,
  .setup-actions button { width: auto; }
  .inline-actions .btn,
  .inline-actions a.btn { flex: 0 1 auto; }
  .meta-row {
    grid-template-columns: minmax(120px, 0.85fr) minmax(0, 1.15fr);
    gap: 14px;
  }
  .usage-hero {
    grid-template-columns: auto minmax(0, 1fr);
    gap: 18px;
  }
  .ring { width: 112px; height: 112px; }
  .ring-inner { width: 80px; height: 80px; }
  .stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .login-wrap {
    min-height: calc(100dvh - 56px);
    place-items: center;
    align-content: center;
  }
  .login-card { padding: 24px; }
}

/* Desktop */
@media (min-width: 900px) {
  :root {
    --pad-x: 28px;
    --space: 20px;
    --radius: 22px;
    --shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
  }
  main { padding-top: 40px; padding-bottom: 72px; }
  .topbar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 24px;
  }
  .actions { justify-content: flex-end; }
  h1 { font-size: clamp(2.2rem, 4vw, 3.3rem); }
  .grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .card-wide { grid-column: 1 / -1; }
  .setup-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
  .stat-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); }
  .card { padding: 22px; }
  .empty-panel { padding: 28px; }
  .usage-value { font-size: 1.35rem; }
}

@media (prefers-reduced-motion: reduce) {
  *, *::before, *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
`

func writeHTMLHead(w http.ResponseWriter, title string) {
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <title>%s</title>
  <style>%s</style>
</head>
<body>
`, title, uiStyles)
}

func writeHTMLFoot(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, `</body></html>`)
}
