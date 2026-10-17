package brands

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/WokeWorks/adx/server/core"
)

// Copy checking. Marketing copy carries figures: a discount, a starting
// price. Each is a claim, and a claim is only usable when an approved fact
// or an approved offer says the same thing. This reads the figures out of
// text deterministically and judges each one; what it can't read it says it
// can't read, rather than passing it.

// ClaimCheck is one figure found in copy and how it stands.
type ClaimCheck struct {
	Text   string `json:"text"`
	Kind   string `json:"kind"`   // percent, price
	Status string `json:"status"` // approved, unapproved
	FactID *int64 `json:"factId"`
	Detail string `json:"detail"`
}

// ClaimReport is what a piece of copy was checked against and the result.
type ClaimReport struct {
	BrandID      int64  `json:"brandId"`
	OfferID      *int64 `json:"offerId"`
	OfferVersion *int64 `json:"offerVersion"`
	OfferState   string `json:"offerState"`
	// OfferCurrent is the offer's current version, so a caller can see a
	// change since the copy was built.
	OfferCurrent *int64       `json:"offerCurrent"`
	FactIDs      []int64      `json:"factIds"`
	Claims       []ClaimCheck `json:"claims"`
}

var (
	rePercent = regexp.MustCompile(`(\d{1,3}(?:\.\d+)?)\s*[%٪]`)
	rePrice   = regexp.MustCompile(`(?i)(?:AED|د\.إ|درهم)\s*([\d,]+(?:\.\d+)?)\s*(M|million|K|k|مليون)?|([\d,]+(?:\.\d+)?)\s*(M|million|مليون)\b`)
	arDigits  = strings.NewReplacer("٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4", "٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9", "٫", ".", "٬", ",")
)

// CheckCopy judges every percentage and price in text against the brand's
// approved claims for the project and, when the copy was built on an
// offer, that offer version's approved terms. Viewers may: it reads only.
func (b *Brands) CheckCopy(ctx context.Context, actor core.Actor, brandID, projectID, offerID, offerVersion int64, text string) (ClaimReport, error) {
	if !actor.AtLeast(core.RoleViewer) || actor.WorkspaceID == 0 {
		return ClaimReport{}, core.ErrForbidden
	}
	r := ClaimReport{BrandID: brandID, FactIDs: []int64{}, Claims: []ClaimCheck{}}
	approvedPct := map[string]int64{}
	approvedPrice := map[int64]int64{}
	if brandID != 0 {
		claims, err := b.Claims(ctx, actor, brandID, projectID, b.clock.Now())
		if err != nil {
			return ClaimReport{}, err
		}
		for _, c := range claims.Claims {
			r.FactIDs = append(r.FactIDs, c.ID)
			switch c.Kind {
			case KindPercent:
				approvedPct[normNumber(c.Value)] = c.ID
			case KindMoney:
				if n, err := strconv.ParseInt(c.Value, 10, 64); err == nil {
					approvedPrice[n] = c.ID
				}
			}
		}
	}
	var offerPct *float64
	if offerID != 0 {
		oid := offerID
		r.OfferID = &oid
		if offerVersion != 0 {
			ov := offerVersion
			r.OfferVersion = &ov
		}
		if _, versions, err := b.OfferVersions(ctx, actor, offerID); err == nil && len(versions) > 0 {
			cur := versions[len(versions)-1].Version
			r.OfferCurrent = &cur
			for _, v := range versions {
				if v.Version == offerVersion {
					r.OfferState = string(v.State)
					if v.State == OfferApproved {
						offerPct = v.Document.DiscountPct
						if v.Document.PriceFrom != nil {
							approvedPrice[v.Document.PriceFrom.Minor] = 0
						}
					}
				}
			}
		}
	}
	text = arDigits.Replace(text)
	for _, m := range rePercent.FindAllStringSubmatch(text, -1) {
		cc := ClaimCheck{Text: strings.TrimSpace(m[0]), Kind: "percent", Status: "unapproved", Detail: "no approved fact or approved offer states this percentage"}
		if id, ok := approvedPct[normNumber(m[1])]; ok {
			cc.Status, cc.FactID, cc.Detail = "approved", &id, "matches an approved fact"
		} else if offerPct != nil && normNumber(m[1]) == normNumber(strconv.FormatFloat(*offerPct, 'f', -1, 64)) {
			cc.Status, cc.Detail = "approved", "matches the approved offer"
		}
		r.Claims = append(r.Claims, cc)
	}
	for _, m := range rePrice.FindAllStringSubmatch(text, -1) {
		num, unit := m[1], m[2]
		if num == "" {
			num, unit = m[3], m[4]
		}
		minor, ok := priceMinor(num, unit)
		cc := ClaimCheck{Text: strings.TrimSpace(m[0]), Kind: "price", Status: "unapproved", Detail: "no approved price fact or approved offer states this amount"}
		if ok {
			if id, found := approvedPrice[minor]; found {
				cc.Status, cc.Detail = "approved", "matches an approved price"
				if id != 0 {
					cc.FactID = &id
				}
			}
		} else {
			cc.Detail = "the amount could not be read"
		}
		r.Claims = append(r.Claims, cc)
	}
	return r, nil
}

func normNumber(s string) string {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	if err != nil {
		return strings.TrimSpace(s)
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func priceMinor(num, unit string) (int64, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(num, ",", ""), 64)
	if err != nil {
		return 0, false
	}
	switch strings.ToLower(unit) {
	case "m", "million", "مليون":
		f *= 1_000_000
	case "k":
		f *= 1_000
	}
	return int64(f*100 + 0.5), true
}
