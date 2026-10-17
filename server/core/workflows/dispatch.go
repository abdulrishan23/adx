package workflows

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/policies"
	"github.com/WokeWorks/adx/server/core/proposals"
)

// ReadTool answers one read capability.
type ReadTool func(ctx context.Context, actor core.Actor, input map[string]any) (any, error)

// CoreDispatcher maps capability names to core functions. Reads are the
// functions given; propose_change is proposals.Propose, which runs policy
// and leaves the change for a person; there is no other write.
type CoreDispatcher struct {
	Reads     map[string]ReadTool
	Proposals *proposals.Proposals
}

func (d *CoreDispatcher) Call(ctx context.Context, actor core.Actor, tool string, input map[string]any) (any, error) {
	if tool == "propose_change" {
		return d.propose(ctx, actor, input)
	}
	if r, ok := d.Reads[tool]; ok {
		return r(ctx, actor, input)
	}
	return nil, fmt.Errorf("tool %q is not available to workflows on this server", tool)
}

// propose turns a tool call into a proposal. The operation key is the
// idempotency key, so a retried step finds its earlier proposal instead of
// making a second one. A denied proposal is reported as the policy's answer.
func (d *CoreDispatcher) propose(ctx context.Context, actor core.Actor, input map[string]any) (any, error) {
	if d.Proposals == nil {
		return nil, errors.New("proposals are not wired; nothing can be proposed")
	}
	in := proposals.ProposalInput{
		AccountID:     toInt(input["accountId"]),
		Action:        policies.WriteAction(str(input["action"])),
		TargetKind:    str(input["targetKind"]),
		TargetRef:     str(input["targetRef"]),
		TargetVersion: str(input["targetVersion"]),
		Reason:        str(input["reason"]),
		AmountMinor:   toInt(input["amountMinor"]),
		Currency:      str(input["currency"]),
		EnvelopeID:    toInt(input["envelopeId"]),
	}
	key := str(input["operationKey"])
	if key == "" {
		return nil, errors.New("a proposing tool needs the step's operation key")
	}
	in.IdempotencyKey = key
	if cur, ok := input["expectedCurrent"]; ok {
		in.ExpectedCurrent, _ = json.Marshal(cur)
	}
	if p, ok := input["proposed"]; ok {
		in.Proposed, _ = json.Marshal(p)
	}
	if in.Reason == "" {
		in.Reason = "proposed by workflow step " + key
	}
	p, created, err := d.Proposals.Propose(ctx, actor, in)
	if err != nil {
		return nil, err
	}
	return map[string]any{"externalRef": "proposal:" + strconv.FormatInt(p.ID, 10), "proposalId": p.ID, "state": p.State, "policyReason": p.PolicyReason, "created": created}, nil
}

// ExternalState implements ExternalChecker over proposals: a proposal is
// settled once executed, rejected, canceled or denied; approved and proposed
// are still waiting on a person or the outbox.
func (d *CoreDispatcher) ExternalState(ctx context.Context, actor core.Actor, ref string) (string, bool, bool, error) {
	if !strings.HasPrefix(ref, "proposal:") || d.Proposals == nil {
		return "", false, false, fmt.Errorf("unknown external ref %q", ref)
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(ref, "proposal:"), 10, 64)
	if err != nil {
		return "", false, false, err
	}
	p, err := d.Proposals.Get(ctx, actor, id)
	if err != nil {
		return "", false, false, err
	}
	switch p.State {
	case "executing", "executed":
		return p.State, true, false, nil
	case "denied", "rejected", "canceled", "stale":
		return p.State, true, true, nil
	}
	return p.State, false, false, nil
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func toInt(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	}
	return 0
}

var (
	_ Dispatcher      = (*CoreDispatcher)(nil)
	_ ExternalChecker = (*CoreDispatcher)(nil)
)
