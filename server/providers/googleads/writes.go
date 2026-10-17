package googleads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/execution"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
)

// Writes. Four actions, implemented from the v26 REST reference and the
// notes in docs/build/api-notes/google-ads.md: create a paused search
// campaign with its own budget, raise or lower a campaign budget, pause a
// campaign. Nothing else is claimed. Every object ADX creates carries the
// marker in its name so a lost response can be found again.

// Capabilities is the honest list.
func (c *Client) Capabilities() []policies.WriteAction {
	return []policies.WriteAction{policies.CampaignCreatePaus, policies.BudgetRaise, policies.BudgetLower, policies.CampaignPause}
}

type createCampaign struct {
	Name             string `json:"name"`
	DailyBudgetMinor int64  `json:"dailyBudgetMinor"`
	ChannelType      string `json:"channelType"`
}

type budgetChange struct {
	DailyBudgetMinor int64 `json:"dailyBudgetMinor"`
}

// Apply performs one action. A network failure after the request left, or a
// 5xx, is uncertain: the change may have landed.
func (c *Client) Apply(ctx context.Context, req execution.ApplyRequest) (execution.ApplyResult, error) {
	if !digits(req.AccountExternalID) {
		return execution.ApplyResult{}, core.Invalid("Google Ads customer id must be digits")
	}
	switch req.Action {
	case policies.CampaignCreatePaus:
		var in createCampaign
		if err := json.Unmarshal(req.Proposed, &in); err != nil || strings.TrimSpace(in.Name) == "" || in.DailyBudgetMinor <= 0 {
			return execution.ApplyResult{}, core.Invalid("a paused campaign needs a name and a daily budget in minor units")
		}
		channel := strings.ToUpper(in.ChannelType)
		if channel == "" {
			channel = "SEARCH"
		}
		if channel != "SEARCH" && channel != "DISPLAY" && channel != "PERFORMANCE_MAX" && channel != "VIDEO" && channel != "DEMAND_GEN" {
			return execution.ApplyResult{}, core.Invalid("channel type %q is not one ADX creates", in.ChannelType)
		}
		cid := req.AccountExternalID
		budgetTemp := "customers/" + cid + "/campaignBudgets/-1"
		name := strings.TrimSpace(in.Name) + " [" + req.Marker + "]"
		body, _ := json.Marshal(map[string]any{"mutateOperations": []map[string]any{
			{"campaignBudgetOperation": map[string]any{"create": map[string]any{"resourceName": budgetTemp, "name": name + " budget", "amountMicros": strconv.FormatInt(in.DailyBudgetMinor*10000, 10), "deliveryMethod": "STANDARD", "explicitlyShared": false}}},
			{"campaignOperation": map[string]any{"create": map[string]any{"name": name, "status": "PAUSED", "advertisingChannelType": channel, "campaignBudget": budgetTemp, "maximizeConversions": map[string]any{},
				"networkSettings": map[string]any{"targetGoogleSearch": true, "targetSearchNetwork": false, "targetContentNetwork": false}, "containsEuPoliticalAdvertising": "DOES_NOT_CONTAIN_EU_POLITICAL_ADVERTISING"}}},
		}})
		var out struct {
			Responses []struct {
				CampaignResult       *struct{ ResourceName string } `json:"campaignResult"`
				CampaignBudgetResult *struct{ ResourceName string } `json:"campaignBudgetResult"`
			} `json:"mutateOperationResponses"`
		}
		if err := c.doWrite(ctx, req.Token, http.MethodPost, "/"+Version+"/customers/"+cid+"/googleAds:mutate", body, &out); err != nil {
			return execution.ApplyResult{}, err
		}
		refs := []proposals.ProviderRef{}
		for _, r := range out.Responses {
			if r.CampaignResult != nil {
				refs = append(refs, proposals.ProviderRef{Platform: "google_ads", Kind: "campaign", ExternalID: lastSegment(r.CampaignResult.ResourceName), At: time.Now().UTC()})
			}
			if r.CampaignBudgetResult != nil {
				refs = append(refs, proposals.ProviderRef{Platform: "google_ads", Kind: "campaign_budget", ExternalID: lastSegment(r.CampaignBudgetResult.ResourceName), At: time.Now().UTC()})
			}
		}
		if len(refs) == 0 {
			return execution.ApplyResult{}, &execution.UncertainError{Cause: errors.New("mutate returned no resource names")}
		}
		return execution.ApplyResult{Refs: refs, Detail: "paused " + channel + " campaign created"}, nil
	case policies.BudgetRaise, policies.BudgetLower:
		var in budgetChange
		if err := json.Unmarshal(req.Proposed, &in); err != nil || in.DailyBudgetMinor <= 0 {
			return execution.ApplyResult{}, core.Invalid("a budget change needs dailyBudgetMinor")
		}
		if !digits(req.TargetRef) {
			return execution.ApplyResult{}, core.Invalid("the target must be a campaign id")
		}
		rows, err := c.search(ctx, req.Token, req.AccountExternalID, c.loginCustomerID, "SELECT campaign.id, campaign.campaign_budget FROM campaign WHERE campaign.id = "+req.TargetRef)
		if err != nil {
			return execution.ApplyResult{}, err
		}
		if len(rows) == 0 || rows[0].Campaign == nil || rows[0].Campaign.CampaignBudget == "" {
			return execution.ApplyResult{}, core.NotFound("campaign %s has no budget resource in this account", req.TargetRef)
		}
		budget := rows[0].Campaign.CampaignBudget
		body, _ := json.Marshal(map[string]any{"operations": []map[string]any{{"update": map[string]any{"resourceName": budget, "amountMicros": strconv.FormatInt(in.DailyBudgetMinor*10000, 10)}, "updateMask": "amount_micros"}}})
		var out struct {
			Results []struct{ ResourceName string } `json:"results"`
		}
		if err := c.doWrite(ctx, req.Token, http.MethodPost, "/"+Version+"/customers/"+req.AccountExternalID+"/campaignBudgets:mutate", body, &out); err != nil {
			return execution.ApplyResult{}, err
		}
		return execution.ApplyResult{Refs: []proposals.ProviderRef{{Platform: "google_ads", Kind: "campaign_budget", ExternalID: lastSegment(budget), At: time.Now().UTC()}}, Detail: fmt.Sprintf("daily budget set to %d minor units", in.DailyBudgetMinor)}, nil
	case policies.CampaignPause:
		if !digits(req.TargetRef) {
			return execution.ApplyResult{}, core.Invalid("the target must be a campaign id")
		}
		rn := "customers/" + req.AccountExternalID + "/campaigns/" + req.TargetRef
		body, _ := json.Marshal(map[string]any{"operations": []map[string]any{{"update": map[string]any{"resourceName": rn, "status": "PAUSED"}, "updateMask": "status"}}})
		var out struct {
			Results []struct{ ResourceName string } `json:"results"`
		}
		if err := c.doWrite(ctx, req.Token, http.MethodPost, "/"+Version+"/customers/"+req.AccountExternalID+"/campaigns:mutate", body, &out); err != nil {
			return execution.ApplyResult{}, err
		}
		return execution.ApplyResult{Refs: []proposals.ProviderRef{{Platform: "google_ads", Kind: "campaign", ExternalID: req.TargetRef, At: time.Now().UTC()}}, Detail: "campaign paused"}, nil
	}
	return execution.ApplyResult{}, core.Invalid("action %s is not implemented for Google Ads", req.Action)
}

// Lookup finds a campaign created with the marker, or checks the target of
// an update. Updates are idempotent on their own; only creates can duplicate.
func (c *Client) Lookup(ctx context.Context, token, customerID, marker string, action policies.WriteAction, targetRef string) (execution.LookupResult, error) {
	if action != policies.CampaignCreatePaus {
		if !digits(targetRef) {
			return execution.LookupResult{}, core.Invalid("the target must be a campaign id")
		}
		rows, err := c.search(ctx, token, customerID, c.loginCustomerID, "SELECT campaign.id, campaign.status FROM campaign WHERE campaign.id = "+targetRef)
		if err != nil {
			return execution.LookupResult{}, err
		}
		if len(rows) == 0 || rows[0].Campaign == nil {
			return execution.LookupResult{Found: false, Detail: "campaign not found"}, nil
		}
		// An update either applied or not; the dispatcher resends it, which
		// is safe for budgets and status.
		return execution.LookupResult{Found: false, Status: rows[0].Campaign.Status, Detail: "update outcome unknown; resend is safe"}, nil
	}
	rows, err := c.search(ctx, token, customerID, c.loginCustomerID, "SELECT campaign.id, campaign.name, campaign.status FROM campaign WHERE campaign.name LIKE '%["+strings.ReplaceAll(marker, "'", "")+"]%'")
	if err != nil {
		return execution.LookupResult{}, err
	}
	if len(rows) == 0 || rows[0].Campaign == nil {
		return execution.LookupResult{Found: false, Detail: "no campaign carries marker " + marker}, nil
	}
	return execution.LookupResult{Found: true, Status: rows[0].Campaign.Status, Detail: "campaign " + rows[0].Campaign.ID + " carries the marker", Refs: []proposals.ProviderRef{{Platform: "google_ads", Kind: "campaign", ExternalID: rows[0].Campaign.ID, At: time.Now().UTC()}}}, nil
}

// Verify reads the campaign behind the refs back.
func (c *Client) Verify(ctx context.Context, token, customerID string, refs []proposals.ProviderRef) (execution.VerifyResult, error) {
	for _, r := range refs {
		if r.Kind != "campaign" || !digits(r.ExternalID) {
			continue
		}
		rows, err := c.search(ctx, token, customerID, c.loginCustomerID, "SELECT campaign.id, campaign.status FROM campaign WHERE campaign.id = "+r.ExternalID)
		if err != nil {
			return execution.VerifyResult{}, err
		}
		if len(rows) == 0 || rows[0].Campaign == nil {
			return execution.VerifyResult{Exists: false, Detail: "campaign " + r.ExternalID + " not found"}, nil
		}
		return execution.VerifyResult{Exists: true, Status: rows[0].Campaign.Status, Detail: "campaign " + r.ExternalID + " exists"}, nil
	}
	return execution.VerifyResult{Exists: false, Detail: "no campaign ref to verify"}, nil
}

// doWrite is do with the uncertainty rule: transport errors after sending
// and 5xx answers are uncertain, 4xx answers are refusals.
func (c *Client) doWrite(ctx context.Context, token, method, path string, body []byte, out any) error {
	err := c.do(ctx, token, c.loginCustomerID, method, path, body, out)
	if err == nil {
		return nil
	}
	var ne net.Error
	if errors.As(err, &ne) || errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "request failed") {
		return &execution.UncertainError{Cause: err}
	}
	var ce *core.Error
	if errors.As(err, &ce) && ce.Kind == core.KindUnavailable {
		return &execution.UncertainError{Cause: err}
	}
	return err
}

func lastSegment(resourceName string) string {
	if i := strings.LastIndex(resourceName, "/"); i >= 0 {
		return resourceName[i+1:]
	}
	return resourceName
}
