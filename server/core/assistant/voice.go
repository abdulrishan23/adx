package assistant

import (
	"context"
	"regexp"
	"strings"

	"github.com/WokeWorks/adx/server/core"
)

// ActionPreview is what a spoken request looks like it asks for, shown
// before anything happens. Speech is a transcript of the operator, through
// the same read-only tools and permissions as typing; a request that sounds
// like a change is named here and still goes nowhere but the proposals path.
type ActionPreview struct {
	Detected bool     `json:"detected"`
	Intents  []string `json:"intents"`
	// Figures are numbers heard in the transcript, which speech gets wrong
	// most; they are shown back, never acted on.
	Figures []string `json:"figures"`
	Message string   `json:"message"`
}

// VoiceReply is the reply to a spoken turn.
type VoiceReply struct {
	Transcript string        `json:"transcript"`
	Preview    ActionPreview `json:"preview"`
	Reply      Reply         `json:"reply"`
}

var intentPatterns = []struct {
	name string
	re   *regexp.Regexp
}{
	{"budget change", regexp.MustCompile(`(?i)\b(raise|increase|lower|decrease|reduce|cut|set|change|double|halve|bump)\b.*\bbudget`)},
	{"budget change", regexp.MustCompile(`(?i)\bbudget\b.*\b(to|by)\b\s*[\d,.]+`)},
	{"pause", regexp.MustCompile(`(?i)\b(pause|stop|switch off|turn off|disable)\b.*\b(campaign|ad|adset|ad set|ads)\b`)},
	{"resume or launch", regexp.MustCompile(`(?i)\b(resume|enable|turn on|launch|go live|activate|start)\b.*\b(campaign|ad|adset|ad set|ads|plan)\b`)},
	{"bid or targeting change", regexp.MustCompile(`(?i)\b(bid|bids|targeting|audience|cpa|roas)\b.*\b(change|set|raise|lower|increase|decrease|to)\b`)},
	{"send a message", regexp.MustCompile(`(?i)\b(send|text|message|whatsapp|email)\b.*\b(him|her|them|lead|customer|person|everyone)\b`)},
	{"place a call", regexp.MustCompile(`(?i)\b(call|ring|phone)\b.*\b(him|her|them|lead|customer|person|back)\b`)},
	{"approve or reject", regexp.MustCompile(`(?i)\b(approve|reject|accept|sign off|execute|run)\b.*\b(proposal|plan|draft|change|it)\b`)},
	{"edit records", regexp.MustCompile(`(?i)\b(delete|remove|merge|edit|update|mark)\b.*\b(lead|person|contact|record|fact|policy)\b`)},
}

var figurePattern = regexp.MustCompile(`\b\d[\d,]*(\.\d+)?\s*(k|m|aed|dirhams|percent|%)?\b|\b(one|two|three|four|five|six|seven|eight|nine|ten|twenty|thirty|forty|fifty|hundred|thousand|million)\b(\s+(hundred|thousand|million|k|percent))?`)

// Preview names the changes a transcript sounds like it asks for. It is a
// reading of words, not an interpretation of intent: anything it names is
// shown to the person as "this would be a change; nothing was done".
func Preview(transcript string) ActionPreview {
	p := ActionPreview{Intents: []string{}, Figures: []string{}}
	seen := map[string]bool{}
	for _, ip := range intentPatterns {
		if ip.re.MatchString(transcript) && !seen[ip.name] {
			seen[ip.name] = true
			p.Intents = append(p.Intents, ip.name)
		}
	}
	for _, m := range figurePattern.FindAllString(transcript, -1) {
		m = strings.TrimSpace(m)
		if m != "" && !seen["fig:"+m] {
			seen["fig:"+m] = true
			p.Figures = append(p.Figures, m)
		}
	}
	p.Detected = len(p.Intents) > 0
	switch {
	case p.Detected && len(p.Figures) > 0:
		p.Message = "This sounds like a change (" + strings.Join(p.Intents, ", ") + ") with figures heard as " + strings.Join(p.Figures, ", ") + ". Nothing was executed: spoken figures are not trusted, and changes go through proposals and approvals in ADX. The question was answered read-only."
	case p.Detected:
		p.Message = "This sounds like a change (" + strings.Join(p.Intents, ", ") + "). Nothing was executed; changes go through proposals and approvals in ADX. The question was answered read-only."
	default:
		p.Message = "Read-only question; answered through the same tools as typing."
	}
	return p
}

// SendVoice handles one spoken turn: the transcript is previewed, recorded
// as the person's turn with the preview on it, and answered exactly as a
// typed message would be. The model never sees the audio, only the text,
// and the tools it can call are the same read-only allowlist.
func (a *Assistant) SendVoice(ctx context.Context, actor core.Actor, conversationID int64, transcript string) (VoiceReply, error) {
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return VoiceReply{}, core.Invalid("the transcript is empty; nothing was heard")
	}
	preview := Preview(transcript)
	warnings := []string{"voice_transcript"}
	if preview.Detected {
		warnings = append(warnings, "action_preview: "+strings.Join(preview.Intents, ", ")+" (not executed)")
	}
	reply, err := a.send(ctx, actor, conversationID, transcript, warnings)
	if err != nil {
		return VoiceReply{}, err
	}
	return VoiceReply{Transcript: transcript, Preview: preview, Reply: reply}, nil
}
