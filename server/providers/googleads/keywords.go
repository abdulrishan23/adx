package googleads

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/research"
)

// geoTargets maps a country code to Google's geo target constant. Country
// constants are 2000 plus the ISO 3166-1 numeric code. Only the markets
// Reportage sells into are listed; others are refused rather than guessed.
var geoTargets = map[string]int{"AE": 2784, "SA": 2682, "QA": 2634, "KW": 2414, "OM": 2512, "BH": 2048, "EG": 2818, "GB": 2826, "IN": 2356, "PK": 2586, "US": 2840}

// languageConstants maps a language code to Google's language constant.
var languageConstants = map[string]int{"en": 1000, "ar": 1019}

// KeywordIdeas asks Keyword Planner for ideas around the seeds in a country
// and language. Volumes and bids are Google's estimates and are returned as
// such; the caller labels them estimated. Bids arrive in micros of the
// account's currency and are converted to minor units.
func (c *Client) KeywordIdeas(ctx context.Context, token, customerID string, seeds []string, country, language string) ([]research.KeywordIdea, error) {
	if !digits(customerID) {
		return nil, core.Invalid("Google Ads customer id must be digits")
	}
	geo, ok := geoTargets[strings.ToUpper(country)]
	if !ok {
		return nil, core.Invalid("keyword ideas are not configured for country %q", country)
	}
	lang, ok := languageConstants[strings.ToLower(language)]
	if !ok {
		return nil, core.Invalid("keyword ideas are not configured for language %q", language)
	}
	clean := make([]string, 0, len(seeds))
	for _, s := range seeds {
		if s = strings.TrimSpace(s); s != "" {
			clean = append(clean, s)
		}
	}
	if len(clean) == 0 || len(clean) > 20 {
		return nil, core.Invalid("between 1 and 20 keyword seeds are needed")
	}
	body, _ := json.Marshal(map[string]any{
		"keywordSeed":          map[string]any{"keywords": clean},
		"geoTargetConstants":   []string{fmt.Sprintf("geoTargetConstants/%d", geo)},
		"language":             fmt.Sprintf("languageConstants/%d", lang),
		"keywordPlanNetwork":   "GOOGLE_SEARCH",
		"includeAdultKeywords": false,
		"pageSize":             200,
	})
	var out struct {
		Results []struct {
			Text    string `json:"text"`
			Metrics *struct {
				AvgMonthlySearches   string `json:"avgMonthlySearches"`
				Competition          string `json:"competition"`
				CompetitionIndex     string `json:"competitionIndex"`
				LowTopOfPageBidMicro string `json:"lowTopOfPageBidMicros"`
				HighTopOfPageBidMicr string `json:"highTopOfPageBidMicros"`
			} `json:"keywordIdeaMetrics"`
		} `json:"results"`
	}
	if err := c.do(ctx, token, c.loginCustomerID, http.MethodPost, "/"+Version+"/customers/"+customerID+":generateKeywordIdeas", body, &out); err != nil {
		return nil, err
	}
	ideas := make([]research.KeywordIdea, 0, len(out.Results))
	for _, r := range out.Results {
		k := research.KeywordIdea{Text: r.Text}
		if m := r.Metrics; m != nil {
			k.AvgMonthlySearches = parseInt64(m.AvgMonthlySearches)
			k.Competition = m.Competition
			k.CompetitionIndex = parseInt64(m.CompetitionIndex)
			k.LowTopOfPageBidMinor = bidMinor(parseInt64(m.LowTopOfPageBidMicro))
			k.HighTopOfPageBidMinor = bidMinor(parseInt64(m.HighTopOfPageBidMicr))
		}
		ideas = append(ideas, k)
	}
	return ideas, nil
}

func parseInt64(s string) *int64 {
	if s == "" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}

func bidMinor(m *int64) *int64 {
	if m == nil {
		return nil
	}
	v := *m / 10000
	return &v
}
