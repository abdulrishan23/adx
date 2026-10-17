package outreach

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// Combine joins several messengers into one that routes each message by
// channel. A channel two adapters both serve goes to the first given, so the
// operator's order decides; a channel nobody serves is refused before any
// adapter is called. With no adapters it returns nil, which Send reads as
// "no messaging adapter is wired".
func Combine(ms ...Messenger) Messenger {
	r := &router{by: map[string]Messenger{}}
	for _, m := range ms {
		if m == nil {
			continue
		}
		r.names = append(r.names, m.Name())
		for _, ch := range m.Channels() {
			if _, taken := r.by[ch]; !taken {
				r.by[ch] = m
			}
		}
	}
	if len(r.names) == 0 {
		return nil
	}
	if len(r.names) == 1 {
		return ms[0]
	}
	return r
}

type router struct {
	names []string
	by    map[string]Messenger
}

func (r *router) Name() string { return strings.Join(r.names, "+") }

func (r *router) Channels() []string {
	out := make([]string, 0, len(r.by))
	for ch := range r.by {
		out = append(out, ch)
	}
	sort.Strings(out)
	return out
}

func (r *router) Send(ctx context.Context, msg OutboundMessage) (string, error) {
	m, ok := r.by[msg.Channel]
	if !ok {
		return "", fmt.Errorf("no adapter serves channel %q", msg.Channel)
	}
	return m.Send(ctx, msg)
}
