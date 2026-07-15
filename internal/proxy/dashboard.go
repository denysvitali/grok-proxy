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
	Styles        template.CSS
}

func (s *Server) dashboard(w http.ResponseWriter, request *http.Request) {
	setDashboardHeaders(w)
	page := dashboardPage{
		ProxyRows: s.proxyRows(),
		ProxyHost: request.Host,
		Styles:    template.CSS(uiStyles),
	}
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
		observeDashboardFailure("account")
		s.log.WithError(accountErr).WithField("component", "account").Warn("dashboard upstream request failed")
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
		observeDashboardFailure("billing")
		s.log.WithError(billingErr).WithField("component", "billing").Warn("dashboard upstream request failed")
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

var dashboardTemplate = template.Must(template.New("dashboard").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Grok Proxy</title>
  <style>{{.Styles}}</style>
</head>
<body>
<main>
  <div class="shell">
    <header class="topbar">
      <div class="brand">
        <div class="brand-mark">
          <div class="logo">G</div>
          <div class="eyebrow">Proxy online</div>
        </div>
        <h1>Grok Proxy</h1>
        <p class="lede">Account, subscription usage, and local service status for Claude Code and Codex.</p>
      </div>
      <nav class="actions" aria-label="Dashboard actions">
        <a class="btn btn-ghost" href="/">Refresh</a>
        <a class="btn btn-ghost" href="/healthz">Health</a>
        {{if .LoggedIn}}<a class="btn" href="/login">Sign in again</a>{{end}}
      </nav>
    </header>

    {{if .LoggedIn}}
    <div class="grid">
      <section class="card">
        <div class="card-header">
          <div class="section-label"><span class="dot"></span>Account</div>
        </div>
        {{if .AccountError}}
          <div class="notice notice-error">{{.AccountError}}</div>
        {{else}}
          {{if .AccountNotice}}<div class="notice">{{.AccountNotice}}</div>{{end}}
          <div class="identity">{{.AccountName}}</div>
          <p class="subline">Signed-in identity and organization details from Grok Build.</p>
          <dl class="meta-list">
            {{range .AccountRows}}
            <div class="meta-row"><dt>{{.Label}}</dt><dd>{{.Value}}</dd></div>
            {{end}}
          </dl>
        {{end}}
      </section>

      <section class="card">
        <div class="card-header">
          <div class="section-label"><span class="dot"></span>Usage</div>
        </div>
        {{if .UsageError}}
          <div class="notice notice-error">{{.UsageError}}</div>
        {{else}}
          {{if .Usage.HasPercent}}
          <div class="usage-hero">
            <div class="ring" style="--value: {{.Usage.PercentValue}};" aria-hidden="true">
              <div class="ring-inner">
                <div>
                  <div class="usage-value">{{.Usage.Percent}}</div>
                  <div class="usage-caption">used</div>
                </div>
              </div>
            </div>
            <div class="usage-copy">
              <h3>Credit usage this period</h3>
              <p>Live billing data from the same services used by Grok Build.</p>
              <div class="pill-row">
                <span class="pill"><strong>{{.Usage.Percent}}</strong> of included credits</span>
              </div>
            </div>
          </div>
          {{end}}
          <dl class="meta-list">
            {{range .Usage.Rows}}
            <div class="meta-row"><dt>{{.Label}}</dt><dd>{{.Value}}</dd></div>
            {{end}}
          </dl>
        {{end}}
      </section>

      <section class="card card-wide">
        <div class="card-header">
          <div class="section-label"><span class="dot"></span>Proxy</div>
        </div>
        <div class="stat-grid">
          {{range .ProxyRows}}
          <div class="stat">
            <div class="stat-label">{{.Label}}</div>
            <div class="stat-value">{{.Value}}</div>
          </div>
          {{end}}
        </div>
      </section>
    </div>
    {{else}}
    <section class="card card-wide empty">
      <div class="card-header">
        <div class="section-label"><span class="dot"></span>Account</div>
      </div>
      <div class="empty-panel">
        <div class="empty-kicker">Authentication required</div>
        <div class="identity">Sign in to view your usage</div>
        <p class="lede">Connect your xAI account to load subscription, credit, and account information for this proxy.</p>
        <form method="post" action="/login">
          <button class="btn-primary primary" type="submit">Sign in with xAI</button>
        </form>
      </div>
    </section>
    {{end}}

    <section class="card card-wide setup">
      <div class="card-header">
        <div class="section-label"><span class="dot"></span>Claude Code</div>
      </div>
      <p class="lede">Point Claude Code at this proxy, then copy and run the command. Include a port if the hostname needs one.</p>
      <div class="setup-grid">
        <div class="setup-block">
          <label for="proxy-host">Proxy hostname</label>
          <input id="proxy-host" type="text" value="{{.ProxyHost}}" autocomplete="off" spellcheck="false">
        </div>
        <div class="setup-block">
          <label for="claude-command">Launch command</label>
          <textarea id="claude-command" readonly aria-label="Claude Code configuration command"></textarea>
          <div class="setup-actions">
            <button id="copy-claude-command" type="button">Copy command</button>
            <span id="copy-status" class="copy-status" aria-live="polite"></span>
          </div>
        </div>
      </div>
      <p class="subline" style="margin-bottom:0">If client authentication is enabled, replace <code>local</code> with the configured API key.</p>
    </section>

    <footer class="footer">Usage is fetched directly from the same account services used by Grok Build.</footer>
  </div>
</main>
<script>
  const proxyHost = document.getElementById("proxy-host");
  const claudeCommand = document.getElementById("claude-command");
  const copyClaudeCommand = document.getElementById("copy-claude-command");
  const copyStatus = document.getElementById("copy-status");
  function updateClaudeCommand() {
    claudeCommand.value = "ANTHROPIC_BASE_URL=" + window.location.protocol + "//" + proxyHost.value.trim() + " \\\nANTHROPIC_AUTH_TOKEN=local \\\nclaude";
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
