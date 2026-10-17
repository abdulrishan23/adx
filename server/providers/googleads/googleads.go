// Package googleads reads Google Ads over its REST API (v26, see
// docs/build/api-notes/google-ads.md). Developer tokens were sunset on
// 2026-09-09 and are not sent; access comes from the Google Cloud project
// behind the OAuth client. int64 fields arrive as JSON strings and money as
// micros (1/1,000,000 of a currency unit), both converted here.
package googleads

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/reporting"
)

const (
	defaultBaseURL = "https://googleads.googleapis.com"
	// Version is the major version the client targets. Minor versions update
	// it in place.
	Version = "v26"
	// Scope is the OAuth scope Google Ads needs.
	Scope = "https://www.googleapis.com/auth/adwords"
)

type Config struct {
	// LoginCustomerID is the manager account the OAuth user reaches client
	// accounts through, digits only. Empty when the user is on the account
	// directly.
	LoginCustomerID string
	// BaseURL and HTTPClient are for provider tests.
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	baseURL         string
	loginCustomerID string
	http            *http.Client
}

func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(base, "/"), loginCustomerID: strings.ReplaceAll(cfg.LoginCustomerID, "-", ""), http: httpClient}
}

func (c *Client) Platform() string { return "google_ads" }

// ListAccounts lists the customers the token reaches directly, then walks
// each manager one level down with customer_client, so a manager's client
// accounts appear with their currency and time zone.
func (c *Client) ListAccounts(ctx context.Context, token string) ([]reporting.ProviderAccount, error) {
	var accessible struct {
		ResourceNames []string `json:"resourceNames"`
	}
	if err := c.do(ctx, token, "", http.MethodGet, "/"+Version+"/customers:listAccessibleCustomers", nil, &accessible); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []reporting.ProviderAccount
	for _, rn := range accessible.ResourceNames {
		id := strings.TrimPrefix(rn, "customers/")
		rows, err := c.search(ctx, token, id, id, `SELECT customer_client.id, customer_client.descriptive_name, customer_client.level, customer_client.manager,
			customer_client.currency_code, customer_client.time_zone, customer_client.status
			FROM customer_client WHERE customer_client.level <= 1 AND customer_client.status = 'ENABLED'`)
		if err != nil {
			// A customer the user can see but not query (cancelled, no
			// permission) is skipped, not fatal: the others still connect.
			var ce *core.Error
			if errors.As(err, &ce) && ce.Kind == core.KindConflict {
				continue
			}
			return nil, err
		}
		for _, r := range rows {
			cc := r.CustomerClient
			if cc == nil || seen[cc.ID] {
				continue
			}
			seen[cc.ID] = true
			parent := ""
			if cc.Level == "1" {
				parent = id
			}
			out = append(out, reporting.ProviderAccount{ExternalID: cc.ID, Name: cc.DescriptiveName, Currency: cc.CurrencyCode, TimeZone: cc.TimeZone,
				ParentID: parent, Manager: cc.Manager, Status: cc.Status})
		}
	}
	return out, nil
}

// ListCampaigns reads every campaign with its budget. searchStream is
// unpaged, so the result is one page.
func (c *Client) ListCampaigns(ctx context.Context, token, customerID string, cursor string) (reporting.Page[reporting.Campaign], error) {
	if cursor != "" {
		return reporting.Page[reporting.Campaign]{}, errors.New("google ads campaign reads are not paged")
	}
	rows, err := c.search(ctx, token, customerID, c.loginCustomerID, `SELECT campaign.id, campaign.name, campaign.status, campaign.advertising_channel_type,
		campaign_budget.amount_micros, campaign_budget.period, customer.currency_code
		FROM campaign WHERE campaign.status != 'REMOVED'`)
	if err != nil {
		return reporting.Page[reporting.Campaign]{}, err
	}
	page := reporting.Page[reporting.Campaign]{Items: make([]reporting.Campaign, 0, len(rows))}
	for _, r := range rows {
		if r.Campaign == nil {
			continue
		}
		cmp := reporting.Campaign{ExternalID: r.Campaign.ID, Name: r.Campaign.Name, Status: r.Campaign.Status, ChannelType: r.Campaign.AdvertisingChannelType}
		if r.Customer != nil {
			cmp.Currency = r.Customer.CurrencyCode
		}
		if r.CampaignBudget != nil && r.CampaignBudget.AmountMicros != "" {
			minor, err := microsToMinor(r.CampaignBudget.AmountMicros, cmp.Currency)
			if err == nil {
				cmp.BudgetMinor = &minor
				cmp.BudgetPeriod = strings.ToLower(r.CampaignBudget.Period)
				if cmp.BudgetPeriod == "" {
					cmp.BudgetPeriod = "daily"
				}
			}
		}
		page.Items = append(page.Items, cmp)
	}
	return page, nil
}

// DailyMetrics reads one row per campaign per day. Days are the account's
// time zone (customer.time_zone), which is how Google segments them.
func (c *Client) DailyMetrics(ctx context.Context, token, customerID string, from, to reporting.Day, cursor string) (reporting.Page[reporting.DailyMetric], error) {
	if cursor != "" {
		return reporting.Page[reporting.DailyMetric]{}, errors.New("google ads metric reads are not paged")
	}
	if !validDay(string(from)) || !validDay(string(to)) {
		return reporting.Page[reporting.DailyMetric]{}, core.Invalid("days must be YYYY-MM-DD")
	}
	rows, err := c.search(ctx, token, customerID, c.loginCustomerID, fmt.Sprintf(`SELECT segments.date, campaign.id, customer.currency_code,
		metrics.cost_micros, metrics.impressions, metrics.clicks, metrics.conversions, metrics.conversions_value, metrics.all_conversions
		FROM campaign WHERE segments.date BETWEEN '%s' AND '%s'`, from, to))
	if err != nil {
		return reporting.Page[reporting.DailyMetric]{}, err
	}
	page := reporting.Page[reporting.DailyMetric]{Items: make([]reporting.DailyMetric, 0, len(rows))}
	for _, r := range rows {
		if r.Campaign == nil || r.Segments == nil || r.Metrics == nil {
			continue
		}
		currency := ""
		if r.Customer != nil {
			currency = r.Customer.CurrencyCode
		}
		cost, err := microsToMinor(r.Metrics.CostMicros, currency)
		if err != nil {
			return reporting.Page[reporting.DailyMetric]{}, fmt.Errorf("google ads cost: %w", err)
		}
		m := reporting.DailyMetric{CampaignExternalID: r.Campaign.ID, Day: reporting.Day(r.Segments.Date), Currency: currency, CostMinor: cost,
			Impressions: atoi(r.Metrics.Impressions), Clicks: atoi(r.Metrics.Clicks)}
		if r.Metrics.Conversions != nil {
			v := *r.Metrics.Conversions
			m.Conversions = &v
		}
		if r.Metrics.ConversionsValue != nil {
			minor := majorToMinor(*r.Metrics.ConversionsValue, currency)
			m.ConversionsValue = &minor
		}
		if r.Metrics.AllConversions != nil {
			v := *r.Metrics.AllConversions
			m.AllConversions = &v
		}
		page.Items = append(page.Items, m)
	}
	return page, nil
}

func (c *Client) ListConversionActions(ctx context.Context, token, customerID string) ([]reporting.ConversionAction, error) {
	rows, err := c.search(ctx, token, customerID, c.loginCustomerID, `SELECT conversion_action.id, conversion_action.name, conversion_action.type,
		conversion_action.category, conversion_action.status, conversion_action.counting_type, conversion_action.include_in_conversions_metric
		FROM conversion_action WHERE conversion_action.status != 'REMOVED'`)
	if err != nil {
		return nil, err
	}
	out := make([]reporting.ConversionAction, 0, len(rows))
	for _, r := range rows {
		if r.ConversionAction == nil {
			continue
		}
		a := r.ConversionAction
		out = append(out, reporting.ConversionAction{ExternalID: a.ID, Name: a.Name, Type: a.Type, Category: a.Category, Status: a.Status,
			CountingType: a.CountingType, IncludeInConversions: a.IncludeInConversionsMetric})
	}
	return out, nil
}

// row is one searchStream result in Google's lowerCamelCase JSON mapping.
type row struct {
	Campaign *struct {
		ID                     string `json:"id"`
		Name                   string `json:"name"`
		Status                 string `json:"status"`
		AdvertisingChannelType string `json:"advertisingChannelType"`
		CampaignBudget         string `json:"campaignBudget"`
	} `json:"campaign"`
	CampaignBudget *struct {
		AmountMicros string `json:"amountMicros"`
		Period       string `json:"period"`
	} `json:"campaignBudget"`
	Customer *struct {
		CurrencyCode string `json:"currencyCode"`
		TimeZone     string `json:"timeZone"`
	} `json:"customer"`
	CustomerClient *struct {
		ID              string `json:"id"`
		DescriptiveName string `json:"descriptiveName"`
		Level           string `json:"level"`
		Manager         bool   `json:"manager"`
		CurrencyCode    string `json:"currencyCode"`
		TimeZone        string `json:"timeZone"`
		Status          string `json:"status"`
	} `json:"customerClient"`
	Segments *struct {
		Date string `json:"date"`
	} `json:"segments"`
	Metrics *struct {
		CostMicros       string   `json:"costMicros"`
		Impressions      string   `json:"impressions"`
		Clicks           string   `json:"clicks"`
		Conversions      *float64 `json:"conversions"`
		ConversionsValue *float64 `json:"conversionsValue"`
		AllConversions   *float64 `json:"allConversions"`
	} `json:"metrics"`
	ConversionAction *struct {
		ID                         string `json:"id"`
		Name                       string `json:"name"`
		Type                       string `json:"type"`
		Category                   string `json:"category"`
		Status                     string `json:"status"`
		CountingType               string `json:"countingType"`
		IncludeInConversionsMetric bool   `json:"includeInConversionsMetric"`
	} `json:"conversionAction"`
}

// search runs a GAQL query with searchStream, whose REST body is a JSON
// array of batches each holding results.
func (c *Client) search(ctx context.Context, token, customerID, loginCustomerID, query string) ([]row, error) {
	if !digits(customerID) {
		return nil, core.Invalid("Google Ads customer id must be digits")
	}
	body, _ := json.Marshal(map[string]string{"query": strings.Join(strings.Fields(query), " ")})
	var batches []struct {
		Results   []row  `json:"results"`
		RequestID string `json:"requestId"`
	}
	if err := c.do(ctx, token, loginCustomerID, http.MethodPost, "/"+Version+"/customers/"+customerID+"/googleAds:searchStream", body, &batches); err != nil {
		return nil, err
	}
	var rows []row
	for _, b := range batches {
		rows = append(rows, b.Results...)
	}
	return rows, nil
}

// apiError is Google's status JSON with the GoogleAdsFailure detail.
type apiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
		Details []struct {
			Type   string `json:"@type"`
			Errors []struct {
				ErrorCode map[string]string `json:"errorCode"`
				Message   string            `json:"message"`
				Details   *struct {
					QuotaErrorDetails *struct {
						RetryDelay string `json:"retryDelay"`
					} `json:"quotaErrorDetails"`
				} `json:"details"`
			} `json:"errors"`
			RequestID string `json:"requestId"`
		} `json:"details"`
	} `json:"error"`
}

// RetryAfter is attached to a quota error so the run can wait the delay
// Google asked for.
type RetryAfter struct {
	Delay time.Duration
}

func (r RetryAfter) Error() string { return "retry after " + r.Delay.String() }

func (c *Client) do(ctx context.Context, token, loginCustomerID, method, path string, body []byte, out any) error {
	var rdr io.Reader
	if body != nil {
		rdr = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ADX/0.1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if loginCustomerID != "" && digits(loginCustomerID) {
		req.Header.Set("login-customer-id", loginCustomerID)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("google ads request failed: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return errors.New("cannot read google ads response")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return mapError(res.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.New("google ads returned an invalid response")
	}
	return nil
}

// mapError turns Google's status into a core error with a safe message. The
// provider's message and request id go to the cause (logged), never the user.
func mapError(status int, raw []byte) error {
	var ae apiError
	_ = json.Unmarshal(raw, &ae)
	family, code := "", ""
	var retry time.Duration
	for _, d := range ae.Error.Details {
		for _, e := range d.Errors {
			for f, cd := range e.ErrorCode {
				family, code = f, cd
			}
			if e.Details != nil && e.Details.QuotaErrorDetails != nil {
				if dur, err := time.ParseDuration(strings.TrimSuffix(e.Details.QuotaErrorDetails.RetryDelay, "s") + "s"); err == nil {
					retry = dur
				}
			}
		}
	}
	cause := fmt.Errorf("google ads HTTP %d %s %s.%s: %s", status, ae.Error.Status, family, code, ae.Error.Message)
	switch {
	case family == "authenticationError":
		return &core.Error{Kind: core.KindConflict, Message: "Google Ads authorization expired or was revoked; reconnect", Cause: cause}
	case family == "authorizationError" || status == http.StatusForbidden:
		return &core.Error{Kind: core.KindConflict, Message: "Google Ads access was rejected; check the account permissions and the manager account (" + code + ")", Cause: cause}
	case family == "quotaError" || status == http.StatusTooManyRequests:
		msg := "Google Ads quota exhausted; retry later"
		if retry > 0 {
			return &core.Error{Kind: core.KindConflict, Message: msg, Cause: errors.Join(cause, RetryAfter{Delay: retry})}
		}
		return &core.Error{Kind: core.KindConflict, Message: msg, Cause: cause}
	case status == http.StatusUnauthorized:
		return &core.Error{Kind: core.KindConflict, Message: "Google Ads authorization expired or was revoked; reconnect", Cause: cause}
	case status >= 500:
		return &core.Error{Kind: core.KindUnavailable, Message: "Google Ads is unavailable; retry later", Cause: cause}
	}
	return &core.Error{Kind: core.KindConflict, Message: "Google Ads rejected the request (" + firstNonEmpty(code, ae.Error.Status, strconv.Itoa(status)) + ")", Cause: cause}
}

// minorUnits is how many minor units make one major unit per currency.
// Everything not listed has two decimals (ISO 4217 default).
var minorUnits = map[string]int64{"JPY": 1, "KRW": 1, "KWD": 1000, "BHD": 1000, "OMR": 1000, "IQD": 1000, "JOD": 1000, "TND": 1000}

func unitsFor(currency string) int64 {
	if u, ok := minorUnits[strings.ToUpper(currency)]; ok {
		return u
	}
	return 100
}

// microsToMinor converts Google's micros (1e-6 units) to minor units,
// rounding half away from zero.
func microsToMinor(micros string, currency string) (int64, error) {
	v, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("micros %q is not an integer", micros)
	}
	units := unitsFor(currency)
	// micros * units / 1e6
	q := v * units
	return int64(math.Round(float64(q) / 1e6)), nil
}

func majorToMinor(major float64, currency string) int64 {
	return int64(math.Round(major * float64(unitsFor(currency))))
}

func atoi(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

func digits(s string) bool {
	if s == "" || len(s) > 20 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validDay(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

var _ reporting.Provider = (*Client)(nil)
