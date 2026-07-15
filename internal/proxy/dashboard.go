package proxy

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/denysvitali/grok-proxy/internal/grok"
)

type dashboardRow struct {
	Label string
	Value string
}

type dashboardUsage struct {
	HasPercent   bool
	Percent      string
	PercentValue string
	Rows         []dashboardRow
}

type dashboardPage struct {
	LoggedIn      bool
	AccountName   string
	AccountRows   []dashboardRow
	AccountError  string
	AccountNotice string
	Usage         dashboardUsage
	UsageError    string
	ProxyRows     []dashboardRow
	ProxyHost     string
}

func (s *Server) dashboard(w http.ResponseWriter, request *http.Request) {
	setDashboardHeaders(w)
	page := dashboardPage{ProxyRows: s.proxyRows(), ProxyHost: request.Host}
	if s.tokens == nil || s.tokens.Store == nil {
		s.renderDashboard(w, page)
		return
	}

	token, err := s.tokens.Usable(request.Context())
	if err != nil {
		s.renderDashboard(w, page)
		return
	}
	page.LoggedIn = true

	ctx, cancel := context.WithTimeout(request.Context(), 20*time.Second)
	defer cancel()
	account, accountErr := s.dashboardClient.Account(ctx, token.AccessToken)

	if accountErr != nil {
		s.log.WithError(accountErr).Warn("dashboard account request failed")
		account, accountErr = grok.AccountFromToken(token.AccessToken)
		if accountErr != nil {
			page.AccountError = "Account information is temporarily unavailable."
		} else {
			page.AccountNotice = "Some account details could not be loaded."
			page.AccountName, page.AccountRows = accountView(account)
		}
	} else {
		page.AccountName, page.AccountRows = accountView(account)
	}
	billing, billingErr := s.dashboardClient.Billing(ctx, token.AccessToken, account.UserID)
	if billingErr != nil {
		page.UsageError = "Usage information is temporarily unavailable."
		s.log.WithError(billingErr).Warn("dashboard billing request failed")
	} else if !billing.Available {
		page.UsageError = "No billing data is available for this account."
	} else {
		page.Usage = usageView(billing)
	}
	s.renderDashboard(w, page)
}

func (s *Server) renderDashboard(w http.ResponseWriter, page dashboardPage) {
	var output bytes.Buffer
	if err := dashboardTemplate.Execute(&output, page); err != nil {
		s.log.WithError(err).Error("render dashboard")
		http.Error(w, "failed to render dashboard", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = output.WriteTo(w)
}

func (s *Server) proxyRows() []dashboardRow {
	clientAuth := "Disabled"
	if s.config.Server.APIKey != "" {
		clientAuth = "Enabled"
	}
	return []dashboardRow{
		{Label: "Status", Value: "Online"},
		{Label: "Listen address", Value: valueOrDash(s.config.Server.Listen)},
		{Label: "Default model", Value: valueOrDash(s.config.Proxy.DefaultModel)},
		{Label: "Upstream", Value: valueOrDash(s.config.BaseURL)},
		{Label: "Client authentication", Value: clientAuth},
	}
}

func accountView(account grok.Account) (string, []dashboardRow) {
	name := account.Name()
	if name == "" {
		name = firstDisplay(account.Email, account.UserID, "Signed in")
	}
	rows := make([]dashboardRow, 0, 14)
	addRow(&rows, "Email", account.Email)
	addRow(&rows, "Subscription", account.SubscriptionTier)
	addRow(&rows, "User ID", account.UserID)
	addRow(&rows, "Team", firstDisplay(account.TeamName, account.TeamID, ""))
	addRow(&rows, "Team role", account.TeamRole)
	addRow(&rows, "Organization", firstDisplay(account.OrganizationName, account.OrganizationID, ""))
	addRow(&rows, "Organization role", account.OrganizationRole)
	addRow(&rows, "Principal type", account.PrincipalType)
	addRow(&rows, "Principal ID", account.PrincipalID)
	if account.HasGrokCodeAccess != nil {
		addRow(&rows, "Grok Build access", yesNo(*account.HasGrokCodeAccess))
	}
	if account.CodingDataRetentionOptOut != nil {
		value := "Standard retention"
		if *account.CodingDataRetentionOptOut {
			value = "Opted out"
		}
		addRow(&rows, "Coding data retention", value)
	}
	addRow(&rows, "Account restriction", account.UserBlockedReason)
	if len(account.TeamBlockedReasons) > 0 {
		addRow(&rows, "Team restrictions", strings.Join(account.TeamBlockedReasons, ", "))
	}
	return name, rows
}

func usageView(billing grok.Billing) dashboardUsage {
	view := dashboardUsage{HasPercent: true}
	percent := 0.0
	if billing.CreditUsagePercent.Valid {
		percent = billing.CreditUsagePercent.Value
	} else if billing.MonthlyLimit.Valid && billing.MonthlyLimit.Value > 0 && billing.Used.Valid {
		percent = billing.Used.Value / billing.MonthlyLimit.Value * 100
	}
	percent = max(0, min(100, percent))
	view.Percent = fmt.Sprintf("%.1f%%", percent)
	view.PercentValue = strconv.FormatFloat(percent, 'f', 2, 64)
	addRow(&view.Rows, "Subscription", billing.SubscriptionTier)
	addRow(&view.Rows, "Period", strings.TrimPrefix(billing.CurrentPeriod.Type, "USAGE_PERIOD_TYPE_"))
	addRow(&view.Rows, "Period start", firstDisplay(billing.CurrentPeriod.Start, billing.BillingPeriodStart))
	addRow(&view.Rows, "Period end", firstDisplay(billing.CurrentPeriod.End, billing.BillingPeriodEnd))
	addCentRow(&view.Rows, "Included used", billing.Used)
	addCentRow(&view.Rows, "Included limit", billing.MonthlyLimit)
	addCentRow(&view.Rows, "Extra usage used", billing.OnDemandUsed)
	addCentRow(&view.Rows, "Extra usage cap", billing.OnDemandCap)
	addCentRow(&view.Rows, "Prepaid balance", billing.PrepaidBalance)
	if billing.OnDemandEnabled != nil {
		addRow(&view.Rows, "Extra usage", enabledDisabled(*billing.OnDemandEnabled))
	}
	if billing.IsUnifiedBillingUser != nil {
		addRow(&view.Rows, "Unified billing", yesNo(*billing.IsUnifiedBillingUser))
	}
	return view
}

func addRow(rows *[]dashboardRow, label, value string) {
	if value != "" {
		*rows = append(*rows, dashboardRow{Label: label, Value: value})
	}
}

func addCentRow(rows *[]dashboardRow, label string, value grok.Number) {
	if value.Valid {
		addRow(rows, label, fmt.Sprintf("$%.2f", math.Abs(value.Value)/100))
	}
}

func valueOrDash(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func firstDisplay(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func yesNo(value bool) string {
	if value {
		return "Yes"
	}
	return "No"
}

func enabledDisabled(value bool) string {
	if value {
		return "Enabled"
	}
	return "Disabled"
}

func setDashboardHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Grok Proxy</title>
  <style>
    :root { color-scheme: dark; font-family: Inter, ui-sans-serif, system-ui, sans-serif; background: #09090b; color: #fafafa; }
    * { box-sizing: border-box; }
    body { margin: 0; min-height: 100vh; background: radial-gradient(circle at top, #1c1c22 0, #09090b 45%); }
    main { width: min(1080px, calc(100% - 32px)); margin: 0 auto; padding: 48px 0 72px; }
    header { display: flex; align-items: flex-start; justify-content: space-between; gap: 24px; margin-bottom: 32px; }
    h1, h2, p { margin-top: 0; } h1 { margin-bottom: 8px; font-size: clamp(2rem, 6vw, 3.75rem); letter-spacing: -.06em; }
    h2 { font-size: 1rem; color: #a1a1aa; text-transform: uppercase; letter-spacing: .12em; }
    .muted { color: #a1a1aa; } .status { display: inline-flex; gap: 8px; align-items: center; color: #a7f3d0; }
    .status::before { content: ""; width: 8px; height: 8px; border-radius: 50%; background: #34d399; box-shadow: 0 0 18px #34d399; }
    .actions { display: flex; flex-wrap: wrap; gap: 10px; }
    a, button { color: #fafafa; background: #27272a; border: 1px solid #3f3f46; border-radius: 999px; padding: 10px 16px; text-decoration: none; font: inherit; cursor: pointer; }
    a:hover, button:hover { background: #3f3f46; }
    .primary { background: #fafafa; color: #09090b; border-color: #fafafa; }
    .grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; }
    .card { background: rgba(24,24,27,.88); border: 1px solid #27272a; border-radius: 20px; padding: 24px; box-shadow: 0 18px 60px rgba(0,0,0,.25); }
    .wide { grid-column: 1 / -1; } .identity { font-size: 1.6rem; font-weight: 650; margin-bottom: 20px; }
    dl { margin: 0; } .row { display: grid; grid-template-columns: minmax(120px, .8fr) minmax(0, 1.2fr); gap: 16px; padding: 11px 0; border-top: 1px solid #27272a; }
    dt { color: #a1a1aa; } dd { margin: 0; overflow-wrap: anywhere; }
    .usage { font-size: 3rem; font-weight: 700; letter-spacing: -.05em; margin-bottom: 8px; }
    progress { width: 100%; height: 12px; margin: 4px 0 22px; accent-color: #a78bfa; }
    .notice { padding: 16px; border: 1px solid #713f12; background: #422006; color: #fde68a; border-radius: 12px; }
    .empty { text-align: center; padding: 64px 24px; }
    .setup { display: grid; gap: 12px; } .setup label { color: #a1a1aa; font-size: .875rem; }
    .setup input, .setup textarea { width: 100%; color: #fafafa; background: #09090b; border: 1px solid #3f3f46; border-radius: 10px; padding: 12px; font: .875rem ui-monospace, SFMono-Regular, Menlo, monospace; }
    .setup textarea { min-height: 104px; resize: vertical; } .setup-actions { display: flex; align-items: center; gap: 12px; }
    .copy-status { color: #a1a1aa; font-size: .875rem; }
    footer { color: #71717a; margin-top: 24px; font-size: .875rem; }
    @media (max-width: 720px) { main { padding-top: 28px; } header { display: block; } .actions { margin-top: 20px; } .grid { grid-template-columns: 1fr; } .wide { grid-column: auto; } .row { grid-template-columns: 1fr; gap: 4px; } }
  </style>
</head>
<body>
<main>
  <header>
    <div><div class="status">Proxy online</div><h1>Grok Proxy</h1><p class="muted">Account, usage, and service status.</p></div>
    <nav class="actions"><a href="/">Refresh</a><a href="/healthz">Health</a>{{if .LoggedIn}}<a href="/login">Sign in again</a>{{end}}</nav>
  </header>
  {{if .LoggedIn}}
  <div class="grid">
    <section class="card">
      <h2>Account</h2>
      {{if .AccountError}}<p class="notice">{{.AccountError}}</p>{{else}}{{if .AccountNotice}}<p class="notice">{{.AccountNotice}}</p>{{end}}<div class="identity">{{.AccountName}}</div><dl>{{range .AccountRows}}<div class="row"><dt>{{.Label}}</dt><dd>{{.Value}}</dd></div>{{end}}</dl>{{end}}
    </section>
    <section class="card">
      <h2>Usage</h2>
      {{if .UsageError}}<p class="notice">{{.UsageError}}</p>{{else}}{{if .Usage.HasPercent}}<div class="usage">{{.Usage.Percent}}</div><progress max="100" value="{{.Usage.PercentValue}}">{{.Usage.Percent}}</progress>{{end}}<dl>{{range .Usage.Rows}}<div class="row"><dt>{{.Label}}</dt><dd>{{.Value}}</dd></div>{{end}}</dl>{{end}}
    </section>
    <section class="card wide"><h2>Proxy</h2><dl>{{range .ProxyRows}}<div class="row"><dt>{{.Label}}</dt><dd>{{.Value}}</dd></div>{{end}}</dl></section>
  </div>
  {{else}}
  <section class="card empty"><h2>Account</h2><div class="identity">Sign in to view your usage</div><p class="muted">Connect your xAI account to load subscription, credit, and account information.</p><form method="post" action="/login"><button class="primary" type="submit">Sign in with xAI</button></form></section>
  {{end}}
  <section class="card wide setup">
    <h2>Claude Code</h2>
    <p class="muted">Set the proxy hostname, then copy and run this command. Include a port if needed.</p>
    <label for="proxy-host">Proxy hostname</label>
    <input id="proxy-host" type="text" value="{{.ProxyHost}}" autocomplete="off" spellcheck="false">
    <textarea id="claude-command" readonly aria-label="Claude Code configuration command"></textarea>
    <div class="setup-actions"><button id="copy-claude-command" type="button">Copy command</button><span id="copy-status" class="copy-status" aria-live="polite"></span></div>
    <p class="muted">If client authentication is enabled, replace <code>local</code> with the configured API key.</p>
  </section>
  <footer>Usage is fetched directly from the same account services used by Grok Build.</footer>
</main>
<script>
  const proxyHost = document.getElementById("proxy-host");
  const claudeCommand = document.getElementById("claude-command");
  const copyClaudeCommand = document.getElementById("copy-claude-command");
  const copyStatus = document.getElementById("copy-status");
  function updateClaudeCommand() {
    claudeCommand.value = "ANTHROPIC_BASE_URL=" + window.location.protocol + "//" + proxyHost.value.trim() + " \\\n+ANTHROPIC_AUTH_TOKEN=local \\\n+claude";
  }
  proxyHost.addEventListener("input", updateClaudeCommand);
  copyClaudeCommand.addEventListener("click", async () => {
    try {
      await navigator.clipboard.writeText(claudeCommand.value);
      copyStatus.textContent = "Copied";
    } catch {
      claudeCommand.select();
      document.execCommand("copy");
      copyStatus.textContent = "Copied";
    }
  });
  updateClaudeCommand();
</script>
</body>
</html>`))
