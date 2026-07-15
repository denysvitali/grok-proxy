package grok

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/denysvitali/grok-proxy/internal/config"
)

const (
	accountsURL = "https://accounts.x.ai/user?include=subscription"
	billingURL  = "https://grok.com/billing?format=credits"
)

type DashboardClient struct {
	AccountsURL string
	BillingURL  string
	HTTP        *http.Client
}

func NewDashboardClient(client *http.Client) *DashboardClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &DashboardClient{AccountsURL: accountsURL, BillingURL: billingURL, HTTP: client}
}

type Account struct {
	UserID                    string
	Email                     string
	FirstName                 string
	LastName                  string
	ProfileImageURL           string
	TeamID                    string
	TeamName                  string
	TeamRole                  string
	OrganizationID            string
	OrganizationName          string
	OrganizationRole          string
	PrincipalType             string
	PrincipalID               string
	SubscriptionTier          string
	UserBlockedReason         string
	TeamBlockedReasons        []string
	CodingDataRetentionOptOut *bool
	HasGrokCodeAccess         *bool
}

func (a Account) Name() string {
	return strings.TrimSpace(a.FirstName + " " + a.LastName)
}

type Number struct {
	Value float64
	Valid bool
}

func (n Number) String() string {
	if !n.Valid {
		return ""
	}
	return strconv.FormatFloat(n.Value, 'f', -1, 64)
}

type BillingPeriod struct {
	End              string
	Month            string
	BillingCycle     string
	IncludedUsed     Number
	TotalUsed        Number
	OnDemandEnabled  *bool
	SubscriptionTier string
}

type Billing struct {
	End                  string
	Month                string
	CreditUsagePercent   Number
	CurrentPeriod        BillingPeriod
	MonthlyLimit         Number
	OnDemandCap          Number
	OnDemandUsed         Number
	PrepaidBalance       Number
	IsUnifiedBillingUser *bool
	BillingPeriodStart   string
	SubscriptionTier     string
}

func (c *DashboardClient) Account(ctx context.Context, accessToken string) (Account, error) {
	body, err := c.get(ctx, c.AccountsURL, "cli", false, accessToken)
	if err != nil {
		return Account{}, err
	}
	return decodeAccount(body)
}

func (c *DashboardClient) Billing(ctx context.Context, accessToken string) (Billing, error) {
	body, err := c.get(ctx, c.BillingURL, "billing", true, accessToken)
	if err != nil {
		return Billing{}, err
	}
	return decodeBilling(body)
}

func (c *DashboardClient) get(ctx context.Context, endpoint, mode string, includeVersion bool, accessToken string) ([]byte, error) {
	if accessToken == "" {
		return nil, errors.New("missing access token")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("x-grok-client-mode", mode)
	if includeVersion {
		request.Header.Set("x-grok-client-version", config.ClientVersion)
	}
	response, err := c.HTTP.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request account service: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read account service response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("account service returned HTTP %d", response.StatusCode)
	}
	return body, nil
}

// AccountFromToken recovers the basic identity claims that Grok extracts from
// the OAuth access token before its optional /user enrichment request.
func AccountFromToken(accessToken string) (Account, error) {
	parts := strings.Split(accessToken, ".")
	if len(parts) != 3 || len(parts[1]) > 1<<20 {
		return Account{}, errors.New("access token is not a JWT")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Account{}, fmt.Errorf("decode access token claims: %w", err)
	}
	claims, err := decodeObject(payload)
	if err != nil {
		return Account{}, fmt.Errorf("decode access token claims: %w", err)
	}
	account := accountFromObject(claims)
	if account.UserID == "" && account.Email == "" && account.Name() == "" {
		return Account{}, errors.New("access token contains no account identity claims")
	}
	return account, nil
}

func decodeAccount(body []byte) (Account, error) {
	root, err := decodeObject(body)
	if err != nil {
		return Account{}, fmt.Errorf("decode account data: %w", err)
	}
	data := unwrap(root, "data", "user")
	return accountFromObject(data), nil
}

func accountFromObject(data map[string]any) Account {
	subscription, _ := objectValue(data, "subscription")
	return Account{
		UserID:                    stringValue(data, "userId", "user_id", "id"),
		Email:                     stringValue(data, "email", "emailAddress", "email_address"),
		FirstName:                 stringValue(data, "firstName", "first_name", "givenName", "given_name"),
		LastName:                  stringValue(data, "lastName", "last_name", "familyName", "family_name"),
		ProfileImageURL:           stringValue(data, "profileImageUrl", "profile_image_url", "picture"),
		TeamID:                    stringValue(data, "teamId", "team_id"),
		TeamName:                  stringValue(data, "teamName", "team_name"),
		TeamRole:                  stringValue(data, "teamRole", "team_role"),
		OrganizationID:            stringValue(data, "organizationId", "organization_id"),
		OrganizationName:          stringValue(data, "organizationName", "organization_name"),
		OrganizationRole:          stringValue(data, "organizationRole", "organization_role"),
		PrincipalType:             stringValue(data, "principalType", "principal_type"),
		PrincipalID:               stringValue(data, "principalId", "principal_id"),
		SubscriptionTier:          firstNonEmpty(stringValue(data, "subscriptionTier", "subscription_tier"), stringValue(subscription, "tier", "name", "displayName", "subscriptionTier", "subscription_tier")),
		UserBlockedReason:         stringValue(data, "userBlockedReason", "user_blocked_reason"),
		TeamBlockedReasons:        stringsValue(data, "teamBlockedReasons", "team_blocked_reasons"),
		CodingDataRetentionOptOut: boolValue(data, "codingDataRetentionOptOut", "coding_data_retention_opt_out"),
		HasGrokCodeAccess:         boolValue(data, "hasGrokCodeAccess", "has_grok_code_access"),
	}
}

func decodeBilling(body []byte) (Billing, error) {
	root, err := decodeObject(body)
	if err != nil {
		return Billing{}, fmt.Errorf("decode billing data: %w", err)
	}
	data := unwrap(root, "data", "billing", "credits")
	period, _ := objectValue(data, "currentPeriod", "current_period")
	current := BillingPeriod{
		End:              stringValue(period, "end"),
		Month:            stringValue(period, "month"),
		BillingCycle:     stringValue(period, "billingCycle", "billing_cycle"),
		IncludedUsed:     numberValue(period, "includedUsed", "included_used"),
		TotalUsed:        numberValue(period, "totalUsed", "total_used"),
		OnDemandEnabled:  boolValue(period, "onDemandEnabled", "on_demand_enabled"),
		SubscriptionTier: stringValue(period, "subscriptionTier", "subscription_tier"),
	}
	return Billing{
		End:                  firstNonEmpty(stringValue(data, "end"), current.End),
		Month:                firstNonEmpty(stringValue(data, "month"), current.Month),
		CreditUsagePercent:   numberValue(data, "creditUsagePercent", "credit_usage_percent"),
		CurrentPeriod:        current,
		MonthlyLimit:         numberValue(data, "monthlyLimit", "monthly_limit"),
		OnDemandCap:          numberValue(data, "onDemandCap", "on_demand_cap"),
		OnDemandUsed:         numberValue(data, "onDemandUsed", "on_demand_used"),
		PrepaidBalance:       numberValue(data, "prepaidBalance", "prepaid_balance"),
		IsUnifiedBillingUser: boolValue(data, "isUnifiedBillingUser", "is_unified_billing_user"),
		BillingPeriodStart:   stringValue(data, "billingPeriodStart", "billing_period_start"),
		SubscriptionTier:     firstNonEmpty(stringValue(data, "subscriptionTier", "subscription_tier"), current.SubscriptionTier),
	}, nil
}

func decodeObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, errors.New("expected a JSON object")
	}
	return object, nil
}

func unwrap(value map[string]any, keys ...string) map[string]any {
	for {
		advanced := false
		for _, key := range keys {
			if child, ok := objectValue(value, key); ok {
				value = child
				advanced = true
				break
			}
		}
		if !advanced {
			return value
		}
	}
}

func objectValue(object map[string]any, names ...string) (map[string]any, bool) {
	value, ok := lookup(object, names...)
	if !ok {
		return nil, false
	}
	result, ok := value.(map[string]any)
	return result, ok
}

func stringValue(object map[string]any, names ...string) string {
	value, ok := lookup(object, names...)
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return ""
	}
}

func stringsValue(object map[string]any, names ...string) []string {
	value, ok := lookup(object, names...)
	if !ok {
		return nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok && text != "" {
			result = append(result, text)
		}
	}
	return result
}

func numberValue(object map[string]any, names ...string) Number {
	value, ok := lookup(object, names...)
	if !ok || value == nil {
		return Number{}
	}
	var (
		number float64
		err    error
	)
	switch typed := value.(type) {
	case json.Number:
		number, err = typed.Float64()
	case float64:
		number = typed
	case string:
		number, err = strconv.ParseFloat(typed, 64)
	default:
		return Number{}
	}
	return Number{Value: number, Valid: err == nil}
}

func boolValue(object map[string]any, names ...string) *bool {
	value, ok := lookup(object, names...)
	if !ok || value == nil {
		return nil
	}
	result, ok := value.(bool)
	if !ok {
		return nil
	}
	return &result
}

func lookup(object map[string]any, names ...string) (any, bool) {
	for _, name := range names {
		if value, ok := object[name]; ok {
			return value, true
		}
	}
	for key, value := range object {
		normalized := normalizeKey(key)
		for _, name := range names {
			if normalized == normalizeKey(name) {
				return value, true
			}
		}
	}
	return nil, false
}

func normalizeKey(value string) string {
	return strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
