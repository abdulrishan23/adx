package meta

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/endpoints"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
)

// OAuthConfig is the Meta app. Standard Access on a Business app covers the
// accounts people with a role on the app can reach (D-22).
type OAuthConfig struct {
	AppID       string
	AppSecret   string
	RedirectURL string
	// GraphBaseURL and Endpoint are for tests.
	GraphBaseURL string
	Endpoint     *oauth2.Endpoint
	HTTPClient   *http.Client
	Version      string
}

// OAuth implements connections.AdsIdentity for Meta. The short-lived user
// token from the code exchange is traded for a long-lived one (about 60
// days); Refresh re-exchanges the current token the same way, since Meta
// issues no refresh token.
type OAuth struct {
	cfg     oauth2.Config
	graph   string
	version string
	http    *http.Client
}

func NewOAuth(c OAuthConfig) (*OAuth, error) {
	if c.AppID == "" || c.AppSecret == "" || c.RedirectURL == "" {
		return nil, errors.New("meta app ID, secret and redirect URL are required")
	}
	endpoint := endpoints.Facebook
	if c.Endpoint != nil {
		endpoint = *c.Endpoint
	}
	graph := c.GraphBaseURL
	if graph == "" {
		graph = defaultBaseURL
	}
	v := c.Version
	if v == "" {
		v = Version
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &OAuth{cfg: oauth2.Config{ClientID: c.AppID, ClientSecret: c.AppSecret, RedirectURL: c.RedirectURL, Endpoint: endpoint, Scopes: Scopes},
		graph: strings.TrimRight(graph, "/"), version: v, http: httpClient}, nil
}

func (o *OAuth) AuthorizationURL(state, _ string) string {
	return o.cfg.AuthCodeURL(state)
}

func (o *OAuth) Exchange(ctx context.Context, code, _ string) (connections.AdsTokens, error) {
	ctx = context.WithValue(ctx, oauth2.HTTPClient, o.http)
	tok, err := o.cfg.Exchange(ctx, code)
	if err != nil {
		return connections.AdsTokens{}, core.Unavailable(err, "Meta didn't complete the sign-in, try again")
	}
	long, err := o.longLived(ctx, tok.AccessToken)
	if err != nil {
		return connections.AdsTokens{}, err
	}
	subject, err := o.subject(ctx, long.AccessToken)
	if err != nil {
		return connections.AdsTokens{}, err
	}
	long.Subject = subject
	long.Scopes = Scopes
	return long, nil
}

func (o *OAuth) Refresh(ctx context.Context, current string) (connections.AdsTokens, error) {
	return o.longLived(ctx, current)
}

// longLived is GET /oauth/access_token?grant_type=fb_exchange_token.
func (o *OAuth) longLived(ctx context.Context, short string) (connections.AdsTokens, error) {
	q := url.Values{"grant_type": {"fb_exchange_token"}, "client_id": {o.cfg.ClientID}, "client_secret": {o.cfg.ClientSecret}, "fb_exchange_token": {short}}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.graph+"/"+o.version+"/oauth/access_token?"+q.Encode(), nil)
	if err != nil {
		return connections.AdsTokens{}, err
	}
	res, err := o.http.Do(req)
	if err != nil {
		return connections.AdsTokens{}, core.Unavailable(err, "Meta didn't complete the sign-in, try again")
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusBadRequest || res.StatusCode == http.StatusUnauthorized {
		return connections.AdsTokens{}, core.Conflict("Meta authorization expired or was revoked; reconnect")
	}
	if res.StatusCode != http.StatusOK {
		return connections.AdsTokens{}, core.Unavailable(errors.New("exchange HTTP "+res.Status), "Meta didn't complete the sign-in, try again")
	}
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil || body.AccessToken == "" {
		return connections.AdsTokens{}, core.Unavailable(err, "Meta returned an invalid token response")
	}
	out := connections.AdsTokens{AccessToken: body.AccessToken}
	if body.ExpiresIn > 0 {
		out.ExpiresAt = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	}
	return out, nil
}

func (o *OAuth) subject(ctx context.Context, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.graph+"/"+o.version+"/me?fields=id", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := o.http.Do(req)
	if err != nil {
		return "", core.Unavailable(err, "Meta didn't say who signed in, try again")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return "", core.Unavailable(errors.New("me HTTP "+res.Status), "Meta didn't say who signed in, try again")
	}
	var me struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(res.Body).Decode(&me); err != nil || me.ID == "" {
		return "", core.Unavailable(err, "Meta didn't say who signed in, try again")
	}
	return me.ID, nil
}

var _ connections.AdsIdentity = (*OAuth)(nil)
