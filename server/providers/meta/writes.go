package meta

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
)

// Writes. Three actions from the Marketing API reference noted in
// docs/build/api-notes/meta-marketing.md: create a paused campaign, change
// a campaign's daily budget, pause a campaign. Ad sets, ads, creatives and
// audiences are not claimed. Created campaigns carry the marker in their
// name so a lost response can be found again.

func (c *Client) Capabilities() []policies.WriteAction {
	return []policies.WriteAction{policies.CampaignCreatePaus, policies.BudgetRaise, policies.BudgetLower, policies.CampaignPause}
}

type createCampaign struct {
	Name                string   `json:"name"`
	DailyBudgetMinor    int64    `json:"dailyBudgetMinor"`
	Objective           string   `json:"objective"`
	SpecialAdCategories []string `json:"specialAdCategories"`
}

type budgetChange struct {
	DailyBudgetMinor int64 `json:"dailyBudgetMinor"`
}

func (c *Client) Apply(ctx context.Context, req execution.ApplyRequest) (execution.ApplyResult, error) {
	acct := strings.TrimPrefix(req.AccountExternalID, "act_")
	switch req.Action {
	case policies.CampaignCreatePaus:
		var in createCampaign
		if err := json.Unmarshal(req.Proposed, &in); err != nil || strings.TrimSpace(in.Name) == "" {
			return execution.ApplyResult{}, core.Invalid("a paused campaign needs a name")
		}
		objective := strings.ToUpper(in.Objective)
		if objective == "" {
			objective = "OUTCOME_LEADS"
		}
		cats, _ := json.Marshal(nonNil(in.SpecialAdCategories))
		form := url.Values{"name": {strings.TrimSpace(in.Name) + " [" + req.Marker + "]"}, "objective": {objective}, "status": {"PAUSED"}, "special_ad_categories": {string(cats)}, "buying_type": {"AUCTION"}}
		if in.DailyBudgetMinor > 0 {
			form.Set("daily_budget", strconv.FormatInt(in.DailyBudgetMinor, 10))
		}
		var out struct {
			ID string `json:"id"`
		}
		if err := c.post(ctx, req.Token, "/act_"+acct+"/campaigns", form, &out); err != nil {
			return execution.ApplyResult{}, err
		}
		if out.ID == "" {
			return execution.ApplyResult{}, &execution.UncertainError{Cause: errors.New("create returned no id")}
		}
		return execution.ApplyResult{Refs: []proposals.ProviderRef{{Platform: "meta", Kind: "campaign", ExternalID: out.ID, At: time.Now().UTC()}}, Detail: "paused " + objective + " campaign created"}, nil
	case policies.BudgetRaise, policies.BudgetLower:
		var in budgetChange
		if err := json.Unmarshal(req.Proposed, &in); err != nil || in.DailyBudgetMinor <= 0 {
			return execution.ApplyResult{}, core.Invalid("a budget change needs dailyBudgetMinor")
		}
		if !digitsOnly(req.TargetRef) {
			return execution.ApplyResult{}, core.Invalid("the target must be a campaign id")
		}
		var out struct {
			Success bool `json:"success"`
		}
		if err := c.post(ctx, req.Token, "/"+req.TargetRef, url.Values{"daily_budget": {strconv.FormatInt(in.DailyBudgetMinor, 10)}}, &out); err != nil {
			return execution.ApplyResult{}, err
		}
		return execution.ApplyResult{Refs: []proposals.ProviderRef{{Platform: "meta", Kind: "campaign", ExternalID: req.TargetRef, At: time.Now().UTC()}}, Detail: fmt.Sprintf("daily budget set to %d minor units", in.DailyBudgetMinor)}, nil
	case policies.CampaignPause:
		if !digitsOnly(req.TargetRef) {
			return execution.ApplyResult{}, core.Invalid("the target must be a campaign id")
		}
		var out struct {
			Success bool `json:"success"`
		}
		if err := c.post(ctx, req.Token, "/"+req.TargetRef, url.Values{"status": {"PAUSED"}}, &out); err != nil {
			return execution.ApplyResult{}, err
		}
		return execution.ApplyResult{Refs: []proposals.ProviderRef{{Platform: "meta", Kind: "campaign", ExternalID: req.TargetRef, At: time.Now().UTC()}}, Detail: "campaign paused"}, nil
	}
	return execution.ApplyResult{}, core.Invalid("action %s is not implemented for Meta", req.Action)
}

func (c *Client) Lookup(ctx context.Context, token, accountID, marker string, action policies.WriteAction, targetRef string) (execution.LookupResult, error) {
	if action != policies.CampaignCreatePaus {
		return execution.LookupResult{Found: false, Detail: "update outcome unknown; resend is safe"}, nil
	}
	acct := strings.TrimPrefix(accountID, "act_")
	filter, _ := json.Marshal([]map[string]string{{"field": "name", "operator": "CONTAIN", "value": "[" + marker + "]"}})
	var out struct {
		Data []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := c.get(ctx, token, c.url("/act_"+acct+"/campaigns", url.Values{"fields": {"id,name,status,effective_status"}, "filtering": {string(filter)}}), &out); err != nil {
		return execution.LookupResult{}, err
	}
	if len(out.Data) == 0 {
		return execution.LookupResult{Found: false, Detail: "no campaign carries marker " + marker}, nil
	}
	return execution.LookupResult{Found: true, Status: out.Data[0].Status, Detail: "campaign " + out.Data[0].ID + " carries the marker", Refs: []proposals.ProviderRef{{Platform: "meta", Kind: "campaign", ExternalID: out.Data[0].ID, At: time.Now().UTC()}}}, nil
}

func (c *Client) Verify(ctx context.Context, token, _ string, refs []proposals.ProviderRef) (execution.VerifyResult, error) {
	for _, r := range refs {
		if r.Kind != "campaign" || !digitsOnly(r.ExternalID) {
			continue
		}
		var out struct {
			ID              string `json:"id"`
			Status          string `json:"status"`
			EffectiveStatus string `json:"effective_status"`
		}
		err := c.get(ctx, token, c.url("/"+r.ExternalID, url.Values{"fields": {"id,status,effective_status"}}), &out)
		var ce *core.Error
		if errors.As(err, &ce) && ce.Kind == core.KindNotFound {
			return execution.VerifyResult{Exists: false, Detail: "campaign " + r.ExternalID + " not found"}, nil
		}
		if err != nil {
			return execution.VerifyResult{}, err
		}
		status := out.EffectiveStatus
		if status == "" {
			status = out.Status
		}
		return execution.VerifyResult{Exists: out.ID != "", Status: status, Detail: "campaign " + out.ID + " exists"}, nil
	}
	return execution.VerifyResult{Exists: false, Detail: "no campaign ref to verify"}, nil
}

// post sends a form-encoded write with the bearer token in the header.
// Transport failures after sending and 5xx answers are uncertain.
func (c *Client) post(ctx context.Context, token, path string, form url.Values, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/"+c.version+path, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ADX/0.1")
	res, err := c.http.Do(req)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) {
			return &execution.UncertainError{Cause: err}
		}
		return &execution.UncertainError{Cause: fmt.Errorf("meta request failed: %w", err)}
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return &execution.UncertainError{Cause: errors.New("cannot read meta response")}
	}
	if res.StatusCode >= 500 {
		return &execution.UncertainError{Cause: fmt.Errorf("meta HTTP %d", res.StatusCode)}
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return mapError(res.StatusCode, raw, res.Header)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return &execution.UncertainError{Cause: errors.New("meta returned an invalid response")}
	}
	return nil
}

func digitsOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}
