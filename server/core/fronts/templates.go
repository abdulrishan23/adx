package fronts

// Template is a ready-to-fire prompt, the way a marketer would ask it. The
// first four are the pills under Home's bar; the rest open with "Show more
// templates", grouped by door.
type Template struct {
	ID          string `json:"id"`
	Door        Door   `json:"door"`
	Icon        string `json:"icon"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

var templates = []Template{
	{"t1", DoorPerform, "changes", "What changed since yesterday?", "The daily check in plain words: what moved, by how much, and whether it matters.", "What changed since yesterday?"},
	{"t2", DoorPerform, "cost", "Why did cost per qualified lead change?", "Breaks the change down by brand, channel and campaign, and names the cause.", "Why did cost per qualified lead change this week?"},
	{"t3", DoorPerform, "waste", "Where is money going without leads?", "Ad sets, keywords and search terms spending with nothing to show for it.", "Where is money going without qualified leads?"},
	{"t4", DoorConvert, "call", "Who should sales call first today?", "Hot leads ranked by signal and wait, with the reason for each.", "Which leads should sales call first today?"},
	{"t5", DoorPerform, "pace", "Are we on pace this month?", "Spend against budget by brand and channel, and where the month will land.", "Are we on pace this month?"},
	{"t6", DoorPerform, "compare", "Compare the brands this month", "Spend, qualified leads and cost per qualified, side by side.", "Which project is cheapest per Done-Deal?"},
	{"t7", DoorPerform, "budget", "Which campaigns should get more budget?", "Campaigns qualifying leads below target cost, with room to spend.", "Which campaigns should get more budget?"},
	{"t8", DoorPerform, "exclude", "Search terms to exclude", "Terms with spend and no qualified leads, ready to add as negatives.", "Search terms to exclude"},
	{"t9", DoorConvert, "revive", "Which leads are back in play?", "Cold leads that visited, replied or re-enquired this week.", "Which leads came back in play this week?"},
	{"t10", DoorConvert, "reasons", "Why are leads not qualifying?", "The reasons sales gives most, by brand and source.", "Why are leads not qualifying?"},
	{"t11", DoorConvert, "sources", "Which sources bring buyers?", "Leads, qualified leads and Done-Deals by source, portals included.", "Which sources bring buyers?"},
	{"t12", DoorConvert, "forms", "Are any forms losing leads?", "Platform leads that never reached GoHighLevel, by form, since when.", "Are any forms losing leads before GoHighLevel?"},
	{"t13", DoorStudio, "launch", "What's blocking a launch?", "Every brand with a plan approved and campaigns not yet delivering, and why.", "What's blocking a launch?"},
	{"t14", DoorStudio, "fatigue", "Which ads are fatigued?", "Ads shown too often with click rate falling, and what to swap in.", "Which ads are fatigued?"},
	{"t15", DoorStudio, "review", "Which creatives are waiting on review?", "Ads with no decision on the review board, oldest first.", "Which creatives are waiting on review?"},
	{"t16", DoorStudio, "plan", "Draft a media plan for a brand", "Budget by channel and week from the brand's facts and research.", "Draft a media plan for a brand"},
	{"t17", DoorAny, "report", "Draft the weekly report", "The CMO's Monday summary, from the same figures as the dashboard.", "Draft the weekly report for the CMO."},
	{"t18", DoorAny, "summary", "Summarise the month for the CEO", "Spend, qualified leads, Done-Deals and what changed, in one page.", "Summarise the month for the CEO"},
}

// Templates returns a copy of the catalog.
func Templates() []Template {
	out := make([]Template, len(templates))
	copy(out, templates)
	return out
}
