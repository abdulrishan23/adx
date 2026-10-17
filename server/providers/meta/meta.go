// Package meta reads Meta ad accounts over the Graph API (v26.0, see
// docs/build/api-notes/meta-marketing.md). Numbers come back as strings;
// insights spend is a decimal in the account currency's major units while
// budgets are minor-unit integers. Both are converted to minor units here.
// Google's and Meta's conversion claims are never added together; this
// package reports Meta's lead actions as platform leads, not conversions.
package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/reporting"
)

const (
	defaultBaseURL = "https://graph.facebook.com"
	// Version is pinned; Marketing API versions retire faster than Graph
	// versions, so it is configuration-adjacent.
	Version = "v26.0"
)

// Scopes ADX asks for. ads_read covers insights and objects; the lead
// scopes come with lead retrieval (B1.3/B4) and need Page tokens.
var Scopes = []string{"ads_read", "business_management"}

type Config struct {
	BaseURL    string
	HTTPClient *http.Client
	// Version overrides the pinned Graph version.
	Version string
}

type Client struct {
	baseURL string
	version string
	http    *http.Client
}

func New(cfg Config) *Client {
	base := cfg.BaseURL
	if base == "" {
		base = defaultBaseURL
	}
	v := cfg.Version
	if v == "" {
		v = Version
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(base, "/"), version: v, http: httpClient}
}

func (c *Client) Platform() string { return "meta" }

type adAccount struct {
	ID            string `json:"id"`
	AccountID     string `json:"account_id"`
	Name          string `json:"name"`
	Currency      string `json:"currency"`
	TimezoneName  string `json:"timezone_name"`
	AccountStatus int    `json:"account_status"`
	Business      *struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"business"`
}

var accountStatus = map[int]string{1: "ACTIVE", 2: "DISABLED", 3: "UNSETTLED", 7: "PENDING_RISK_REVIEW", 8: "PENDING_SETTLEMENT", 9: "IN_GRACE_PERIOD", 100: "PENDING_CLOSURE", 101: "CLOSED"}

// ListAccounts lists the ad accounts the user token reaches.
func (c *Client) ListAccounts(ctx context.Context, token string) ([]reporting.ProviderAccount, error) {
	var out []reporting.ProviderAccount
	next := c.url("/me/adaccounts", url.Values{"fields": {"id,account_id,name,currency,timezone_name,account_status,business"}, "limit": {"100"}})
	for next != "" {
		var page struct {
			Data   []adAccount `json:"data"`
			Paging paging      `json:"paging"`
		}
		if err := c.get(ctx, token, next, &page); err != nil {
			return nil, err
		}
		for _, a := range page.Data {
			acct := reporting.ProviderAccount{ExternalID: a.ID, Name: a.Name, Currency: a.Currency, TimeZone: a.TimezoneName, Status: accountStatus[a.AccountStatus]}
			if acct.Status == "" {
				acct.Status = strconv.Itoa(a.AccountStatus)
			}
			if a.Business != nil {
				acct.ParentID = a.Business.ID
			}
			out = append(out, acct)
		}
		next = page.Paging.Next
	}
	return out, nil
}

type paging struct {
	Cursors struct {
		After string `json:"after"`
	} `json:"cursors"`
	Next string `json:"next"`
}

type campaign struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Status         string `json:"status"`
	EffectiveState string `json:"effective_status"`
	Objective      string `json:"objective"`
	DailyBudget    string `json:"daily_budget"`
	LifetimeBudget string `json:"lifetime_budget"`
}

// ListCampaigns pages through the account's campaigns. The cursor is the
// `paging.next` URL, which Meta says to follow rather than rebuild.
func (c *Client) ListCampaigns(ctx context.Context, token, accountID string, cursor string) (reporting.Page[reporting.Campaign], error) {
	accountID, err := actID(accountID)
	if err != nil {
		return reporting.Page[reporting.Campaign]{}, err
	}
	target := cursor
	if target == "" {
		target = c.url("/"+accountID+"/campaigns", url.Values{
			"fields":           {"id,name,status,effective_status,objective,daily_budget,lifetime_budget"},
			"effective_status": {`["ACTIVE","PAUSED","IN_PROCESS","WITH_ISSUES","ARCHIVED"]`},
			"limit":            {"100"},
		})
	} else if err := c.sameHost(target); err != nil {
		return reporting.Page[reporting.Campaign]{}, err
	}
	var page struct {
		Data   []campaign `json:"data"`
		Paging paging     `json:"paging"`
	}
	if err := c.get(ctx, token, target, &page); err != nil {
		return reporting.Page[reporting.Campaign]{}, err
	}
	out := reporting.Page[reporting.Campaign]{Items: make([]reporting.Campaign, 0, len(page.Data)), Next: page.Paging.Next}
	for _, cp := range page.Data {
		cmp := reporting.Campaign{ExternalID: cp.ID, Name: cp.Name, Status: cp.EffectiveState, ChannelType: cp.Objective}
		if cmp.Status == "" {
			cmp.Status = cp.Status
		}
		// Budgets are minor-unit integer strings in the account currency.
		// A campaign without a budget runs on ad-set budgets: nil, not zero.
		if cp.DailyBudget != "" && cp.DailyBudget != "0" {
			if v, err := strconv.ParseInt(cp.DailyBudget, 10, 64); err == nil {
				cmp.BudgetMinor, cmp.BudgetPeriod = &v, "daily"
			}
		} else if cp.LifetimeBudget != "" && cp.LifetimeBudget != "0" {
			if v, err := strconv.ParseInt(cp.LifetimeBudget, 10, 64); err == nil {
				cmp.BudgetMinor, cmp.BudgetPeriod = &v, "lifetime"
			}
		}
		out.Items = append(out.Items, cmp)
	}
	return out, nil
}

type insightRow struct {
	CampaignID  string       `json:"campaign_id"`
	DateStart   string       `json:"date_start"`
	DateStop    string       `json:"date_stop"`
	Spend       string       `json:"spend"`
	Impressions string       `json:"impressions"`
	Clicks      string       `json:"clicks"`
	Reach       string       `json:"reach"`
	Actions     []actionStat `json:"actions"`
	Currency    string       `json:"account_currency"`
}

type actionStat struct {
	ActionType string `json:"action_type"`
	Value      string `json:"value"`
}

// leadActionTypes are Meta's lead counts. `lead` is the total of offsite and
// on-Facebook leads, so only it is summed; the grouped types are kept out to
// avoid counting a lead twice.
const leadActionType = "lead"

// DailyMetrics reads campaign-level insights one day at a time
// (time_increment=1). Numbers are strings; spend is a decimal in major units.
func (c *Client) DailyMetrics(ctx context.Context, token, accountID string, from, to reporting.Day, cursor string) (reporting.Page[reporting.DailyMetric], error) {
	accountID, err := actID(accountID)
	if err != nil {
		return reporting.Page[reporting.DailyMetric]{}, err
	}
	if _, err := from.Time(); err != nil {
		return reporting.Page[reporting.DailyMetric]{}, core.Invalid("days must be YYYY-MM-DD")
	}
	if _, err := to.Time(); err != nil {
		return reporting.Page[reporting.DailyMetric]{}, core.Invalid("days must be YYYY-MM-DD")
	}
	target := cursor
	if target == "" {
		target = c.url("/"+accountID+"/insights", url.Values{
			"level":          {"campaign"},
			"time_increment": {"1"},
			"time_range":     {fmt.Sprintf(`{"since":"%s","until":"%s"}`, from, to)},
			"fields":         {"campaign_id,date_start,date_stop,spend,impressions,clicks,reach,actions,account_currency"},
			"limit":          {"500"},
		})
	} else if err := c.sameHost(target); err != nil {
		return reporting.Page[reporting.DailyMetric]{}, err
	}
	var page struct {
		Data   []insightRow `json:"data"`
		Paging paging       `json:"paging"`
	}
	if err := c.get(ctx, token, target, &page); err != nil {
		return reporting.Page[reporting.DailyMetric]{}, err
	}
	out := reporting.Page[reporting.DailyMetric]{Items: make([]reporting.DailyMetric, 0, len(page.Data)), Next: page.Paging.Next}
	for _, r := range page.Data {
		cost, err := decimalToMinor(r.Spend, r.Currency)
		if err != nil {
			return reporting.Page[reporting.DailyMetric]{}, fmt.Errorf("meta spend: %w", err)
		}
		m := reporting.DailyMetric{CampaignExternalID: r.CampaignID, Day: reporting.Day(r.DateStart), Currency: r.Currency, CostMinor: cost,
			Impressions: atoi(r.Impressions), Clicks: atoi(r.Clicks)}
		if r.Reach != "" {
			v := atoi(r.Reach)
			m.Reach = &v
		}
		for _, a := range r.Actions {
			if a.ActionType == leadActionType {
				if v, err := strconv.ParseFloat(a.Value, 64); err == nil {
					m.PlatformLeads = &v
				}
			}
		}
		out.Items = append(out.Items, m)
	}
	return out, nil
}

// ListConversionActions: Meta has pixel events, not conversion actions. The
// Conversions API audit (later phase) lists events; nothing here.
func (c *Client) ListConversionActions(context.Context, string, string) ([]reporting.ConversionAction, error) {
	return nil, nil
}

func (c *Client) url(path string, q url.Values) string {
	return c.baseURL + "/" + c.version + path + "?" + q.Encode()
}

// sameHost refuses to follow a paging URL that points anywhere but the API,
// since the token goes with it.
func (c *Client) sameHost(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return core.Invalid("Meta paging cursor is invalid")
	}
	base, _ := url.Parse(c.baseURL)
	if u.Host != base.Host || u.Scheme != base.Scheme {
		return core.Invalid("Meta paging cursor points at another host")
	}
	return nil
}

type apiError struct {
	Error struct {
		Message     string `json:"message"`
		Type        string `json:"type"`
		Code        int    `json:"code"`
		Subcode     int    `json:"error_subcode"`
		FBTraceID   string `json:"fbtrace_id"`
		UserTitle   string `json:"error_user_title"`
		UserMessage string `json:"error_user_msg"`
		IsTransient bool   `json:"is_transient"`
	} `json:"error"`
}

// Usage is Meta's rate-limit header, kept so a run can pause before being
// throttled.
type Usage struct {
	CallCount, TotalCPUTime, TotalTime int
	EstimatedTimeToRegainAccess        int // minutes
	AccessTier                         string
}

// get sends the token as a bearer header, never in the URL, so it can't land
// in a log. It handles Meta's error shape and the usage headers.
func (c *Client) get(ctx context.Context, token, target string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ADX/0.1")
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("meta request failed: %w", err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return errors.New("cannot read meta response")
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return mapError(res.StatusCode, raw, res.Header)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return errors.New("meta returned an invalid response")
	}
	return nil
}

// ThrottledError carries how long Meta asked us to wait.
type ThrottledError struct{ Wait time.Duration }

func (t ThrottledError) Error() string { return "throttled; wait " + t.Wait.String() }

func mapError(status int, raw []byte, headers http.Header) error {
	var ae apiError
	_ = json.Unmarshal(raw, &ae)
	e := ae.Error
	cause := fmt.Errorf("meta HTTP %d code %d subcode %d type %s: %s (fbtrace %s)", status, e.Code, e.Subcode, e.Type, e.Message, e.FBTraceID)
	switch {
	case e.Code == 190 || e.Code == 102 || status == http.StatusUnauthorized:
		return &core.Error{Kind: core.KindConflict, Message: "Meta authorization expired or was revoked; reconnect", Cause: cause}
	case e.Code == 10 || (e.Code >= 200 && e.Code <= 299) || status == http.StatusForbidden:
		return &core.Error{Kind: core.KindConflict, Message: fmt.Sprintf("Meta access was rejected; check the app roles and permissions (code %d)", e.Code), Cause: cause}
	case e.Code == 4 || e.Code == 17 || e.Code == 32 || e.Code == 613 || e.Code == 80000 || e.Code == 80004 || status == http.StatusTooManyRequests:
		wait := regainAccess(headers)
		return &core.Error{Kind: core.KindConflict, Message: "Meta rate limit reached; retry later", Cause: errors.Join(cause, ThrottledError{Wait: wait})}
	case e.Code == 100 && e.Subcode == 1487534:
		return &core.Error{Kind: core.KindConflict, Message: "Meta says the report is too large; narrow the dates or level", Cause: cause}
	case e.Code == 1 || e.Code == 2 || e.IsTransient || status >= 500:
		return &core.Error{Kind: core.KindUnavailable, Message: "Meta is unavailable; retry later", Cause: cause}
	}
	return &core.Error{Kind: core.KindConflict, Message: fmt.Sprintf("Meta rejected the request (code %d)", e.Code), Cause: cause}
}

// regainAccess reads x-business-use-case-usage for the longest
// estimated_time_to_regain_access, in minutes, or defaults to 5.
func regainAccess(h http.Header) time.Duration {
	raw := h.Get("x-business-use-case-usage")
	wait := 5 * time.Minute
	if raw == "" {
		return wait
	}
	var usage map[string][]struct {
		EstimatedTimeToRegainAccess int `json:"estimated_time_to_regain_access"`
	}
	if err := json.Unmarshal([]byte(raw), &usage); err != nil {
		return wait
	}
	for _, entries := range usage {
		for _, u := range entries {
			if d := time.Duration(u.EstimatedTimeToRegainAccess) * time.Minute; d > wait {
				wait = d
			}
		}
	}
	return wait
}

// ParseUsage reads the business-use-case header for the given account id
// (digits, without act_), for the health screen.
func ParseUsage(h http.Header, accountDigits string) (Usage, bool) {
	raw := h.Get("x-business-use-case-usage")
	if raw == "" {
		return Usage{}, false
	}
	var usage map[string][]struct {
		Type                        string `json:"type"`
		CallCount                   int    `json:"call_count"`
		TotalCPUTime                int    `json:"total_cputime"`
		TotalTime                   int    `json:"total_time"`
		EstimatedTimeToRegainAccess int    `json:"estimated_time_to_regain_access"`
		Tier                        string `json:"ads_api_access_tier"`
	}
	if err := json.Unmarshal([]byte(raw), &usage); err != nil {
		return Usage{}, false
	}
	for _, u := range usage[accountDigits] {
		if u.Type == "ads_insights" {
			return Usage{CallCount: u.CallCount, TotalCPUTime: u.TotalCPUTime, TotalTime: u.TotalTime, EstimatedTimeToRegainAccess: u.EstimatedTimeToRegainAccess, AccessTier: u.Tier}, true
		}
	}
	return Usage{}, false
}

var minorUnits = map[string]int64{"JPY": 1, "KRW": 1, "KWD": 1000, "BHD": 1000, "OMR": 1000, "IQD": 1000, "JOD": 1000, "TND": 1000}

// decimalToMinor converts a decimal string of major units ("5339.5") into
// minor units exactly, with no float rounding of the input.
func decimalToMinor(s, currency string) (int64, error) {
	if s == "" {
		return 0, nil
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return 0, fmt.Errorf("spend %q is not a number", s)
	}
	units := int64(100)
	if u, ok := minorUnits[strings.ToUpper(currency)]; ok {
		units = u
	}
	r.Mul(r, new(big.Rat).SetInt64(units))
	f, _ := r.Float64()
	return int64(math.Round(f)), nil
}

func atoi(s string) int64 {
	v, _ := strconv.ParseInt(s, 10, 64)
	return v
}

// actID normalizes an ad account id to the act_ form Meta uses in paths.
func actID(id string) (string, error) {
	id = strings.TrimSpace(id)
	digitsOnly := strings.TrimPrefix(id, "act_")
	if digitsOnly == "" || len(digitsOnly) > 30 {
		return "", core.Invalid("Meta ad account id is required")
	}
	for _, r := range digitsOnly {
		if r < '0' || r > '9' {
			return "", core.Invalid("Meta ad account id must be act_ followed by digits")
		}
	}
	return "act_" + digitsOnly, nil
}

var _ reporting.Provider = (*Client)(nil)
