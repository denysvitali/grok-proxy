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

const uiStyles = `
:root {
  color-scheme: dark;
  --bg: #07080c;
  --bg-elevated: rgba(18, 20, 28, 0.82);
  --bg-soft: rgba(255, 255, 255, 0.03);
  --border: rgba(255, 255, 255, 0.08);
  --border-strong: rgba(255, 255, 255, 0.14);
  --text: #f4f6fb;
  --muted: #9aa3b5;
  --faint: #6d7588;
  --accent: #8b7cf6;
  --accent-2: #46d6c0;
  --accent-soft: rgba(139, 124, 246, 0.16);
  --ok: #34d399;
  --ok-soft: rgba(52, 211, 153, 0.14);
  --warn: #fbbf24;
  --warn-soft: rgba(251, 191, 36, 0.12);
  --danger: #fb7185;
  --danger-soft: rgba(251, 113, 133, 0.12);
  --shadow: 0 24px 80px rgba(0, 0, 0, 0.45);
  --radius: 22px;
  --radius-sm: 14px;
  --font: "Segoe UI", Inter, ui-sans-serif, system-ui, -apple-system, sans-serif;
  --mono: ui-monospace, "SFMono-Regular", Menlo, Consolas, monospace;
}
* { box-sizing: border-box; }
html { scroll-behavior: smooth; }
body {
  margin: 0;
  min-height: 100vh;
  font-family: var(--font);
  color: var(--text);
  background:
    radial-gradient(1000px 520px at 12% -10%, rgba(139, 124, 246, 0.22), transparent 55%),
    radial-gradient(820px 460px at 92% 0%, rgba(70, 214, 192, 0.14), transparent 50%),
    radial-gradient(700px 420px at 50% 110%, rgba(99, 102, 241, 0.12), transparent 55%),
    linear-gradient(180deg, #0b0d13 0%, var(--bg) 48%, #06070b 100%);
}
body::before {
  content: "";
  position: fixed;
  inset: 0;
  pointer-events: none;
  background-image:
    linear-gradient(rgba(255,255,255,0.025) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255,255,255,0.025) 1px, transparent 1px);
  background-size: 48px 48px;
  mask-image: radial-gradient(circle at center, black, transparent 78%);
  opacity: 0.5;
}
a { color: inherit; }
main {
  position: relative;
  width: min(1120px, calc(100% - 32px));
  margin: 0 auto;
  padding: 40px 0 72px;
}
.shell {
  display: grid;
  gap: 20px;
}
.topbar {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 24px;
  margin-bottom: 8px;
}
.brand {
  display: grid;
  gap: 10px;
}
.brand-mark {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.logo {
  width: 38px;
  height: 38px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  background:
    linear-gradient(145deg, rgba(139,124,246,0.95), rgba(70,214,192,0.85));
  box-shadow: 0 10px 30px rgba(139, 124, 246, 0.28);
  font-weight: 800;
  letter-spacing: -0.04em;
  color: #0b0d13;
}
.eyebrow {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--ok);
  font-size: 0.84rem;
  font-weight: 600;
  letter-spacing: 0.02em;
}
.eyebrow::before {
  content: "";
  width: 8px;
  height: 8px;
  border-radius: 999px;
  background: var(--ok);
  box-shadow: 0 0 0 5px var(--ok-soft), 0 0 18px rgba(52, 211, 153, 0.7);
  animation: pulse 2.4s ease-in-out infinite;
}
h1, h2, h3, p { margin: 0; }
h1 {
  font-size: clamp(2.1rem, 5vw, 3.4rem);
  line-height: 1.02;
  letter-spacing: -0.055em;
  font-weight: 760;
}
.lede {
  max-width: 42rem;
  color: var(--muted);
  font-size: 1.02rem;
  line-height: 1.55;
}
.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  justify-content: flex-end;
}
.btn, button, .chip {
  appearance: none;
  border: 1px solid var(--border-strong);
  background: rgba(255,255,255,0.04);
  color: var(--text);
  border-radius: 999px;
  padding: 11px 16px;
  font: inherit;
  font-size: 0.95rem;
  font-weight: 600;
  text-decoration: none;
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease, transform 0.15s ease, box-shadow 0.15s ease;
}
.btn:hover, button:hover {
  background: rgba(255,255,255,0.08);
  border-color: rgba(255,255,255,0.22);
  transform: translateY(-1px);
}
.btn:active, button:active { transform: translateY(0); }
.btn-primary, button.primary {
  background: linear-gradient(135deg, #f8fafc, #dbe4ff);
  color: #0b1020;
  border-color: transparent;
  box-shadow: 0 10px 28px rgba(219, 228, 255, 0.18);
}
.btn-primary:hover, button.primary:hover {
  background: linear-gradient(135deg, #ffffff, #e8eeff);
}
.btn-ghost {
  background: transparent;
}
.grid {
  display: grid;
  grid-template-columns: repeat(12, minmax(0, 1fr));
  gap: 16px;
}
.card {
  position: relative;
  overflow: hidden;
  grid-column: span 6;
  background:
    linear-gradient(180deg, rgba(255,255,255,0.03), transparent 34%),
    var(--bg-elevated);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: 22px;
  box-shadow: var(--shadow);
  backdrop-filter: blur(18px);
}
.card::after {
  content: "";
  position: absolute;
  inset: 0 auto auto 0;
  width: 180px;
  height: 180px;
  background: radial-gradient(circle, var(--accent-soft), transparent 68%);
  opacity: 0.7;
  pointer-events: none;
}
.card > * { position: relative; z-index: 1; }
.card-wide { grid-column: span 12; }
.card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 18px;
}
.section-label {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: var(--muted);
  font-size: 0.78rem;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}
.section-label .dot {
  width: 7px;
  height: 7px;
  border-radius: 999px;
  background: var(--accent);
  box-shadow: 0 0 0 4px var(--accent-soft);
}
.identity {
  font-size: clamp(1.45rem, 3vw, 1.9rem);
  font-weight: 720;
  letter-spacing: -0.03em;
  margin-bottom: 6px;
}
.subline {
  color: var(--muted);
  margin-bottom: 18px;
  line-height: 1.5;
}
.meta-list {
  display: grid;
  gap: 0;
  margin: 0;
}
.meta-row {
  display: grid;
  grid-template-columns: minmax(120px, 0.85fr) minmax(0, 1.15fr);
  gap: 14px;
  padding: 12px 0;
  border-top: 1px solid rgba(255,255,255,0.06);
}
.meta-row:first-child { border-top: 0; padding-top: 2px; }
.meta-row dt {
  color: var(--muted);
  font-size: 0.92rem;
}
.meta-row dd {
  margin: 0;
  overflow-wrap: anywhere;
  font-weight: 560;
}
.usage-hero {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 20px;
  align-items: center;
  margin-bottom: 18px;
}
.ring {
  --value: 0;
  width: 118px;
  height: 118px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  background:
    radial-gradient(circle at center, rgba(10,12,18,0.96) 58%, transparent 59%),
    conic-gradient(from 210deg, var(--accent) calc(var(--value) * 1%), rgba(255,255,255,0.08) 0);
  box-shadow: inset 0 0 0 1px rgba(255,255,255,0.05);
}
.ring-inner {
  width: 84px;
  height: 84px;
  border-radius: 999px;
  display: grid;
  place-items: center;
  text-align: center;
  background: rgba(8, 10, 16, 0.96);
  border: 1px solid rgba(255,255,255,0.06);
}
.usage-value {
  font-size: 1.35rem;
  font-weight: 760;
  letter-spacing: -0.04em;
}
.usage-caption {
  color: var(--faint);
  font-size: 0.72rem;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}
.usage-copy h3 {
  font-size: 1.15rem;
  margin-bottom: 6px;
}
.usage-copy p {
  color: var(--muted);
  line-height: 1.5;
}
.pill-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 14px;
}
.pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 11px;
  border-radius: 999px;
  background: var(--bg-soft);
  border: 1px solid var(--border);
  color: var(--muted);
  font-size: 0.82rem;
  font-weight: 600;
}
.pill strong { color: var(--text); font-weight: 700; }
.notice {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  padding: 14px 15px;
  border-radius: 14px;
  border: 1px solid rgba(251, 191, 36, 0.28);
  background: var(--warn-soft);
  color: #fde68a;
  margin-bottom: 16px;
  line-height: 1.45;
}
.notice-error {
  border-color: rgba(251, 113, 133, 0.28);
  background: var(--danger-soft);
  color: #fecdd3;
}
.empty {
  text-align: left;
  padding: 8px 2px 4px;
}
.empty-panel {
  display: grid;
  gap: 18px;
  padding: 28px;
  border-radius: calc(var(--radius) - 4px);
  border: 1px dashed rgba(255,255,255,0.12);
  background:
    linear-gradient(180deg, rgba(139,124,246,0.08), transparent 55%),
    rgba(255,255,255,0.02);
}
.empty-kicker {
  color: var(--accent);
  font-size: 0.8rem;
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
}
.setup {
  display: grid;
  gap: 18px;
}
.setup-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 14px;
}
.setup-block {
  display: grid;
  gap: 10px;
  padding: 16px;
  border-radius: var(--radius-sm);
  background: rgba(255,255,255,0.025);
  border: 1px solid rgba(255,255,255,0.06);
}
.setup-block label {
  color: var(--muted);
  font-size: 0.84rem;
  font-weight: 600;
}
.setup input,
.setup textarea {
  width: 100%;
  color: var(--text);
  background: rgba(7, 8, 12, 0.88);
  border: 1px solid rgba(255,255,255,0.1);
  border-radius: 12px;
  padding: 12px 13px;
  font: 0.88rem/1.45 var(--mono);
  outline: none;
  transition: border-color 0.15s ease, box-shadow 0.15s ease;
}
.setup input:focus,
.setup textarea:focus {
  border-color: rgba(139, 124, 246, 0.7);
  box-shadow: 0 0 0 4px rgba(139, 124, 246, 0.14);
}
.setup textarea {
  min-height: 118px;
  resize: vertical;
}
.setup-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
.copy-status {
  color: var(--muted);
  font-size: 0.875rem;
  min-height: 1.2em;
}
.stat-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
}
.stat {
  padding: 14px 15px;
  border-radius: 16px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.06);
}
.stat-label {
  color: var(--muted);
  font-size: 0.8rem;
  margin-bottom: 8px;
}
.stat-value {
  font-size: 1.02rem;
  font-weight: 700;
  letter-spacing: -0.02em;
  overflow-wrap: anywhere;
}
.footer {
  color: var(--faint);
  font-size: 0.875rem;
  line-height: 1.5;
  padding: 4px 4px 0;
}
.login-wrap {
  min-height: calc(100vh - 80px);
  display: grid;
  place-items: center;
}
.login-card {
  width: min(520px, 100%);
  padding: 28px;
}
.login-steps {
  display: grid;
  gap: 12px;
  margin: 22px 0;
}
.step {
  display: grid;
  grid-template-columns: auto 1fr;
  gap: 12px;
  align-items: start;
  padding: 14px;
  border-radius: 14px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.06);
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
}
.stream-log {
  display: grid;
  gap: 10px;
  margin: 18px 0 8px;
}
.stream-item {
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(255,255,255,0.03);
  border: 1px solid rgba(255,255,255,0.06);
  color: var(--muted);
  line-height: 1.45;
}
.stream-item a {
  color: #c4b5fd;
  font-weight: 700;
  text-decoration: none;
}
.stream-item a:hover { text-decoration: underline; }
.success-box, .error-box {
  margin-top: 16px;
  padding: 14px 15px;
  border-radius: 14px;
  line-height: 1.45;
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
  gap: 10px;
  margin-top: 16px;
}
code {
  font-family: var(--mono);
  font-size: 0.9em;
  padding: 0.12em 0.4em;
  border-radius: 7px;
  background: rgba(255,255,255,0.06);
  border: 1px solid rgba(255,255,255,0.06);
}
@keyframes pulse {
  0%, 100% { transform: scale(1); opacity: 1; }
  50% { transform: scale(1.15); opacity: 0.75; }
}
@media (max-width: 860px) {
  .card, .card-wide { grid-column: span 12; }
  .setup-grid, .stat-grid { grid-template-columns: 1fr; }
  .usage-hero { grid-template-columns: 1fr; }
}
@media (max-width: 720px) {
  main { padding-top: 24px; width: min(100% - 20px, 1120px); }
  .topbar { display: grid; gap: 18px; }
  .actions { justify-content: flex-start; }
  .meta-row { grid-template-columns: 1fr; gap: 4px; }
  .card { padding: 18px; border-radius: 18px; }
  .login-card { padding: 22px; }
}
`

func writeHTMLHead(w http.ResponseWriter, title string) {
	_, _ = fmt.Fprintf(w, `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>%s</title>
  <style>%s</style>
</head>
<body>
`, title, uiStyles)
}

func writeHTMLFoot(w http.ResponseWriter) {
	_, _ = fmt.Fprint(w, `</body></html>`)
}
