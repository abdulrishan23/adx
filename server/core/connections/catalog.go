package connections

import "sort"

// ConnectorCapability is one thing a connector can or cannot do for ADX.
// The catalog is written in code, with a reason for everything unsupported,
// so a connector's name on screen never implies more than ADX does with it:
// a Meta logo means "reads reporting and, under policy, changes budgets and
// status", never "publishes posts".
type ConnectorCapability struct {
	Provider   string `json:"provider"`
	Capability string `json:"capability"`
	// Direction is read, write or publish. Publish is a write that puts
	// content in front of the public; ADX does none.
	Direction string `json:"direction"`
	Supported bool   `json:"supported"`
	// Reason says why something is unsupported, or what still gates a
	// supported write (configuration, policy, approval).
	Reason string `json:"reason"`
	// Wired says whether this server has the adapter and configuration the
	// capability needs right now. Supported but unwired is honest: the code
	// exists, the server cannot do it yet.
	Wired bool `json:"wired"`
}

// Wiring is what cmd/adx actually configured. It never holds a secret.
type Wiring struct {
	LiveWrites bool
	// Channels the outreach messenger serves (whatsapp, email).
	Channels []string
	// Telecom is the configured calling route, or empty.
	Telecom string
	Speech  bool
	Model   bool
	// Webhooks is whether GoHighLevel events are accepted.
	Webhooks bool
	// Mail is whether an SMTP relay is configured for reports and links.
	Mail bool
	// Providers lists the ad platforms with app credentials (google_ads, meta)
	// and "gohighlevel" when the CRM client exists.
	Providers []string
}

// Catalog returns every connector capability with its wiring state.
func Catalog(w Wiring) []ConnectorCapability {
	has := func(list []string, v string) bool {
		for _, x := range list {
			if x == v {
				return true
			}
		}
		return false
	}
	ghl, google, meta := has(w.Providers, "gohighlevel"), has(w.Providers, "google_ads"), has(w.Providers, "meta")
	writeGate := "needs ADX_LIVE_WRITES, a policy allowing the action for the account, a verified write capability and, for a plan launch, an approval"
	adsWrites := func(provider string, wired bool) []ConnectorCapability {
		return []ConnectorCapability{
			{provider, "read.reporting", "read", true, "campaigns, daily figures and conversion actions; a missing figure is null", wired},
			{provider, "write.campaign", "write", true, "creates a campaign paused; " + writeGate, wired && w.LiveWrites},
			{provider, "write.budget", "write", true, "raises or lowers a daily budget within caps; " + writeGate, wired && w.LiveWrites},
			{provider, "write.status", "write", true, "pauses a campaign; " + writeGate, wired && w.LiveWrites},
			{provider, "write.audiences", "write", false, "no audience upload adapter; the policy catalog names the action so it can be denied", false},
			{provider, "write.conversions", "write", false, "no offline conversion upload; conversion actions are read only", false},
			{provider, "write.exclusions", "write", false, "no placement or keyword exclusion adapter", false},
			{provider, "publish.creative", "publish", false, "ADX uploads no ads or assets; a reviewed asset is handed to a person who publishes it", false},
		}
	}
	out := []ConnectorCapability{
		{"gohighlevel", "read.leads", "read", true, "contacts, custom fields and pipeline stages, imported into ADX's own records", ghl},
		{"gohighlevel", "read.events", "read", true, "stage changes, replies and form events through the webhook; needs ADX_WEBHOOK_SECRET", ghl && w.Webhooks},
		{"gohighlevel", "write.lead_fields", "write", true, "sets ADX-owned custom fields from the outbox; needs ADX_LIVE_WRITES and the ghl.lead_fields.set policy", ghl && w.LiveWrites},
		{"gohighlevel", "write.messages", "write", false, "ADX does not send through GoHighLevel conversations; outreach goes through its own channel adapters", false},
		{"gohighlevel", "write.pipeline", "write", false, "ADX owns the pipeline natively since B5.1; it reads GoHighLevel stages and never writes them back", false},
		{"google_ads", "read.keyword_ideas", "read", true, "keyword ideas and historical metrics for research", google},
		{"meta", "read.lead_forms", "read", false, "Meta Leads API is not integrated; form leads arrive through the CRM", false},
		{"meta", "read.pixel", "read", false, "no Pixel or Conversions API integration; the measurement check records it as unsupported", false},
		{"meta", "publish.organic", "publish", false, "no publishing to Pages or Instagram", false},
		{"whatsapp", "write.message", "write", true, "sends an approved draft as plain text inside Meta's 24-hour window; needs ADX_LIVE_WRITES, ADX_WHATSAPP_* and the channel mode on", has(w.Channels, "whatsapp")},
		{"whatsapp", "write.template", "write", false, "no template messages; outside the 24-hour window the send fails with Meta's reason", false},
		{"whatsapp", "read.replies", "read", false, "no inbound WhatsApp webhook; replies are observed through the CRM", false},
		{"email", "write.message", "write", true, "sends an approved draft through the SMTP relay; needs ADX_LIVE_WRITES, ADX_SMTP_* and the channel mode on", has(w.Channels, "email")},
		{"email", "read.replies", "read", false, "no mailbox is read", false},
		{"telephony", "voice.call", "write", true, "places a qualification call through the configured route; needs ADX_TELECOM_ROUTE and a calling policy naming the same route", w.Telecom != ""},
		{"telephony", "sms", "write", false, "no SMS adapter", false},
		{"speech", "voice.agent", "write", true, "ElevenLabs conversational agent over the call stream; needs ADX_ELEVENLABS_*", w.Speech},
		{"model", "generate", "write", true, "plans, research, drafts and the assistant; needs ADX_MODEL_API_KEY", w.Model},
		{"mail", "deliver.reports", "write", true, "report and approval mail to staff; needs ADX_SMTP_*", w.Mail},
	}
	out = append(out, adsWrites("google_ads", google)...)
	out = append(out, adsWrites("meta", meta)...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].Capability < out[j].Capability
	})
	return out
}
