// Package gohighlevel implements the HighLevel OAuth and location API client.
package gohighlevel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/connections"
)

type Config struct {
	ClientID, ClientSecret, RedirectURI, VersionID string
	// Overrides are for provider contract tests. The executable uses the defaults.
	APIBaseURL, AuthorizeURL string
	HTTPClient               *http.Client
}

type Client struct {
	clientID, clientSecret, redirectURI, versionID string
	apiBaseURL, authorizeURL                       string
	http                                           *http.Client
}

// NewReader supports a private token without Marketplace client credentials.
func NewReader() *Client {
	return &Client{
		apiBaseURL: "https://services.leadconnectorhq.com",
		http: &http.Client{Timeout: 20 * time.Second,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
	}
}

func New(cfg Config) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, errors.New("GoHighLevel client ID and secret are required")
	}
	u, err := url.Parse(cfg.RedirectURI)
	if err != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.RawQuery != "" {
		return nil, errors.New("GoHighLevel redirect URI is invalid")
	}
	local := u.Hostname() == "localhost"
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		local = ip.IsLoopback()
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !local) {
		return nil, errors.New("GoHighLevel redirect URI must use HTTPS or local HTTP")
	}
	if cfg.APIBaseURL == "" {
		cfg.APIBaseURL = "https://services.leadconnectorhq.com"
	}
	if cfg.AuthorizeURL == "" {
		cfg.AuthorizeURL = "https://marketplace.gohighlevel.com/v2/oauth/chooselocation"
	}
	for _, endpoint := range []string{cfg.APIBaseURL, cfg.AuthorizeURL} {
		u, err := url.Parse(endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" ||
			(u.Scheme != "https" && u.Scheme != "http") {
			return nil, errors.New("GoHighLevel endpoint URL is invalid")
		}
	}
	httpClient := http.Client{Timeout: 20 * time.Second}
	if cfg.HTTPClient != nil {
		httpClient = *cfg.HTTPClient
		if httpClient.Timeout == 0 {
			httpClient.Timeout = 20 * time.Second
		}
	}
	// Never forward credentials through an API redirect.
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{
		clientID: cfg.ClientID, clientSecret: cfg.ClientSecret, redirectURI: cfg.RedirectURI,
		versionID: cfg.VersionID, apiBaseURL: strings.TrimRight(cfg.APIBaseURL, "/"),
		authorizeURL: cfg.AuthorizeURL, http: &httpClient,
	}, nil
}

func (c *Client) AuthorizationURL(state string) string {
	if c.clientID == "" {
		return ""
	}
	u, _ := url.Parse(c.authorizeURL)
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", c.clientID)
	q.Set("redirect_uri", c.redirectURI)
	q.Set("scope", strings.Join(connections.RequiredScopes, " "))
	q.Set("state", state)
	if c.versionID != "" {
		q.Set("version_id", c.versionID)
	}
	u.RawQuery = q.Encode()
	return u.String()
}

func (c *Client) Exchange(ctx context.Context, code string) (connections.Tokens, error) {
	out, err := c.token(ctx, "authorization_code", "code", code)
	if err != nil {
		return connections.Tokens{}, err
	}
	if out.UserType == "Company" {
		return c.locationToken(ctx, out)
	}
	return out.tokens(), nil
}

func (c *Client) Refresh(ctx context.Context, refreshToken string) (connections.Tokens, error) {
	out, err := c.token(ctx, "refresh_token", "refresh_token", refreshToken)
	if err != nil {
		return connections.Tokens{}, err
	}
	return out.tokens(), nil
}

type tokenResponse struct {
	AccessToken              string   `json:"access_token"`
	RefreshToken             string   `json:"refresh_token"`
	TokenType                string   `json:"token_type"`
	ExpiresIn                int64    `json:"expires_in"`
	Scope                    string   `json:"scope"`
	UserType                 string   `json:"userType"`
	LocationID               string   `json:"locationId"`
	CompanyID                string   `json:"companyId"`
	ApprovedLocations        []string `json:"approvedLocations"`
	ApproveAllLocations      bool     `json:"approveAllLocations"`
	InstallToFutureLocations bool     `json:"installToFutureLocations"`
}

func (c *Client) token(ctx context.Context, grant, field, value string) (tokenResponse, error) {
	form := url.Values{
		"client_id": {c.clientID}, "client_secret": {c.clientSecret}, "grant_type": {grant},
		"redirect_uri": {c.redirectURI}, field: {value},
	}
	// The authorization code determines the installer's token type. Only saved
	// location credentials reach Refresh, so its token type is explicit.
	if grant == "refresh_token" {
		form.Set("user_type", "Location")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBaseURL+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return tokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	var out tokenResponse
	if err := c.request(req, &out); err != nil {
		return tokenResponse{}, err
	}
	if !strings.EqualFold(out.TokenType, "Bearer") {
		return tokenResponse{}, errors.New("GoHighLevel returned an unsupported token type")
	}
	return out, nil
}

func (out tokenResponse) tokens() connections.Tokens {
	return connections.Tokens{
		AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, ExpiresIn: out.ExpiresIn,
		Scope: out.Scope, UserType: out.UserType, LocationID: out.LocationID, CompanyID: out.CompanyID,
	}
}

func (c *Client) locationToken(ctx context.Context, agency tokenResponse) (connections.Tokens, error) {
	if agency.CompanyID == "" || agency.AccessToken == "" || agency.ExpiresIn <= 0 {
		return connections.Tokens{}, errors.New("GoHighLevel returned incomplete agency credentials")
	}
	if agency.ApproveAllLocations || agency.InstallToFutureLocations || len(agency.ApprovedLocations) != 1 || agency.ApprovedLocations[0] == "" {
		return connections.Tokens{}, core.Invalid("select only one GoHighLevel workspace, without automatic future installations; start again")
	}
	locationID := agency.ApprovedLocations[0]
	form := url.Values{"companyId": {agency.CompanyID}, "locationId": {locationID}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBaseURL+"/oauth/location-token", strings.NewReader(form.Encode()))
	if err != nil {
		return connections.Tokens{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+agency.AccessToken)
	var out tokenResponse
	if err := c.request(req, &out); err != nil {
		return connections.Tokens{}, err
	}
	if !strings.EqualFold(out.TokenType, "Bearer") || out.LocationID != locationID ||
		(out.CompanyID != "" && out.CompanyID != agency.CompanyID) || (out.UserType != "" && out.UserType != "Location") {
		return connections.Tokens{}, errors.New("GoHighLevel returned credentials for a different workspace")
	}
	// The location-token endpoint can omit these fields. Its location ID must
	// still match the explicitly approved location before we fill its metadata.
	out.UserType, out.CompanyID = "Location", agency.CompanyID
	return out.tokens(), nil
}

func (c *Client) Location(ctx context.Context, accessToken, locationID string) (connections.Location, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+"/locations/"+url.PathEscape(locationID), nil)
	if err != nil {
		return connections.Location{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	var out struct {
		Location struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			CompanyID string `json:"companyId"`
		} `json:"location"`
	}
	if err := c.request(req, &out); err != nil {
		return connections.Location{}, err
	}
	return connections.Location{ID: out.Location.ID, Name: out.Location.Name, CompanyID: out.Location.CompanyID}, nil
}

// ReadAccess checks the remaining read permissions without importing their data.
func (c *Client) ReadAccess(ctx context.Context, token, locationID string) error {
	query := url.Values{"locationId": {locationID}}
	contactsQuery := url.Values{"locationId": {locationID}, "limit": {"1"}}
	paths := []string{
		"/contacts/?" + contactsQuery.Encode(),
		"/locations/" + url.PathEscape(locationID) + "/customFields?model=all",
		"/opportunities/pipelines?" + query.Encode(),
	}
	for _, path := range paths {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBaseURL+path, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		var out json.RawMessage
		if err := c.request(req, &out); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) request(req *http.Request, out any) error {
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "ADX/0.1")
	version := "2021-07-28"
	if strings.HasPrefix(req.URL.Path, "/oauth/") {
		version = "v3"
	}
	req.Header.Set("Version", version)
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("GoHighLevel request failed: %w", err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusBadRequest:
		var oauthError struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&oauthError)
		if oauthError.Error == "invalid_grant" || oauthError.Error == "invalid_token" {
			return core.Conflict("GoHighLevel authorization expired or was revoked; reconnect")
		}
		return errors.New("GoHighLevel request returned HTTP 400")
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return core.Conflict("GoHighLevel access was rejected; check the connection token and permissions")
	case res.StatusCode == http.StatusTooManyRequests:
		return core.Conflict("GoHighLevel request limit reached; try again later")
	case res.StatusCode < 200 || res.StatusCode >= 300:
		// Do not include provider response bodies: they can contain credentials.
		return fmt.Errorf("GoHighLevel request returned HTTP %d", res.StatusCode)
	}
	const maxResponse = 512 << 10
	body, err := io.ReadAll(io.LimitReader(res.Body, maxResponse+1))
	if err != nil {
		return errors.New("cannot read GoHighLevel response")
	}
	if len(body) > maxResponse {
		return errors.New("GoHighLevel response is too large")
	}
	if err := json.Unmarshal(body, out); err != nil {
		return errors.New("GoHighLevel returned an invalid response")
	}
	return nil
}
