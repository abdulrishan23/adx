package campaigns

import (
	"github.com/WokeWorks/adx/server/core/plans"
	"github.com/WokeWorks/adx/server/core/studio"
)

// A campaign's channels decide what it has to make. A Meta lead form needs
// pictures in the feed and story sizes and no page; Google Search needs
// search ad copy and a page; YouTube needs a film. The Ads and Page stages
// read this so a marketer sees what's missing for the channels they picked.

// AssetNeed is one kind of ad a channel runs, and how many the campaign
// has of it. Kind is picture, video or text. Fmt is the studio's size for
// pictures and video; empty for text.
type AssetNeed struct {
	Kind  string `json:"kind"`
	Fmt   string `json:"fmt"`
	Label string `json:"label"`
	// Have counts the campaign's ads of this kind and size, by language.
	Have map[string]int `json:"have"`
	// Want is how many per language the channel needs at least.
	Want int `json:"want"`
}

// ChannelNeed is what one channel asks of the campaign.
type ChannelNeed struct {
	Channel   string      `json:"channel"`
	Label     string      `json:"label"`
	Platform  string      `json:"platform"`
	Supported bool        `json:"supported"`
	Assets    []AssetNeed `json:"assets"`
	// Page says the channel sends people to a landing page.
	Page bool `json:"page"`
	// Note says what Adx can't make for this channel yet.
	Note string `json:"note"`
}

type needSpec struct {
	assets []AssetNeed
	page   bool
	note   string
}

var (
	story     = AssetNeed{Kind: "picture", Fmt: string(studio.FmtStory), Label: "Story 9:16", Want: 1}
	feed      = AssetNeed{Kind: "picture", Fmt: string(studio.FmtFeed), Label: "Feed 4:5", Want: 1}
	square    = AssetNeed{Kind: "picture", Fmt: string(studio.FmtSquare), Label: "Square 1:1", Want: 1}
	landscape = AssetNeed{Kind: "picture", Fmt: string(studio.FmtLandscape), Label: "Landscape 1.91:1", Want: 1}
	film      = AssetNeed{Kind: "video", Fmt: string(studio.FmtStory), Label: "Video 9:16", Want: 1}
	searchAd  = AssetNeed{Kind: "text", Label: "Search ad copy", Want: 1}
)

var needByChannel = map[string]needSpec{
	"meta_lead_form":    {assets: []AssetNeed{story, feed, square}, page: false},
	"meta_traffic":      {assets: []AssetNeed{story, feed, square}, page: true},
	"meta_awareness":    {assets: []AssetNeed{story, feed, film}, page: false},
	"meta_whatsapp":     {assets: []AssetNeed{story, feed}, page: false},
	"google_search":     {assets: []AssetNeed{searchAd}, page: true, note: "Search ad copy isn't made in the studio yet; it's written on the platform."},
	"google_pmax":       {assets: []AssetNeed{square, landscape, feed, searchAd}, page: true, note: "The text part isn't made in the studio yet."},
	"google_display":    {assets: []AssetNeed{landscape, square}, page: true},
	"google_youtube":    {assets: []AssetNeed{film}, page: true},
	"google_demand_gen": {assets: []AssetNeed{square, feed, landscape}, page: true},
	"linkedin":          {assets: []AssetNeed{square, landscape}, page: true, note: "Adx can plan LinkedIn but not launch or measure it; it runs by hand."},
	"tiktok":            {assets: []AssetNeed{film}, page: true, note: "Adx can plan TikTok but not launch or measure it; it runs by hand."},
	"snapchat":          {assets: []AssetNeed{film}, page: true, note: "Adx can plan Snapchat but not launch or measure it; it runs by hand."},
	"programmatic":      {assets: []AssetNeed{landscape, square}, page: true, note: "Runs by hand."},
	"portals":           {assets: []AssetNeed{landscape}, page: false, note: "Listings on Bayut and Property Finder, by hand."},
	"print_outdoor":     {assets: []AssetNeed{}, page: false, note: "Artwork for print and outdoor is made outside Adx."},
}

// needs works out what the campaign's channels ask for and how much of it
// the campaign already has.
func needs(channels []string, concepts []studio.Concept) []ChannelNeed {
	out := []ChannelNeed{}
	for _, key := range channels {
		info, ok := plans.ChannelByKey(key)
		if !ok {
			continue
		}
		spec := needByChannel[key]
		n := ChannelNeed{Channel: key, Label: info.Label, Platform: info.Platform, Supported: info.Supported, Page: spec.page, Note: spec.note, Assets: []AssetNeed{}}
		for _, a := range spec.assets {
			a.Have = map[string]int{"en": 0, "ar": 0}
			for _, k := range concepts {
				for _, ad := range k.Ads {
					if a.Kind == "text" {
						continue
					}
					kind := "picture"
					if ad.Kind == "video" {
						kind = "video"
					}
					if kind == a.Kind && string(ad.Fmt) == a.Fmt && ad.Status != studio.StatusUnpublished {
						a.Have[string(ad.Lang)]++
					}
				}
			}
			n.Assets = append(n.Assets, a)
		}
		out = append(out, n)
	}
	return out
}

// needsPage says whether any channel sends people to a landing page.
func needsPage(needs []ChannelNeed) bool {
	for _, n := range needs {
		if n.Page {
			return true
		}
	}
	return len(needs) == 0
}
