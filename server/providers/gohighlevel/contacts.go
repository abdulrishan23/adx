package gohighlevel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/WokeWorks/adx/server/core"
	"github.com/WokeWorks/adx/server/core/leads"
)

// pageLimit is how many contacts one search returns. The API allows 500;
// 100 keeps each response small and stays well inside the burst limit of
// 100 requests per 10 seconds.
const pageLimit = 100

// searchRequest is POST /contacts/search (API version 2021-07-28). Results
// are sorted by dateAdded (full) or dateUpdated (incremental) and paged with
// searchAfter, the cursor each returned contact carries, because page
// numbers stop at 10,000 records.
type searchRequest struct {
	LocationID  string            `json:"locationId"`
	PageLimit   int               `json:"pageLimit"`
	SearchAfter json.RawMessage   `json:"searchAfter,omitempty"`
	Filters     []searchFilter    `json:"filters,omitempty"`
	Sort        []searchSort      `json:"sort"`
	Page        *int              `json:"page,omitempty"`
	Extra       map[string]string `json:"-"`
}

type searchFilter struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value,omitempty"`
}

type searchSort struct {
	Field     string `json:"field"`
	Direction string `json:"direction"`
}

// searchResponse is what the endpoint returns. Names come back lowercase in
// search results; customFields values can be strings or arrays.
type searchResponse struct {
	Contacts []searchContact `json:"contacts"`
	Total    int64           `json:"total"`
}

type searchContact struct {
	ID                 string            `json:"id"`
	FirstName          string            `json:"firstName"`
	LastName           string            `json:"lastName"`
	FirstNameLower     string            `json:"firstNameLowerCase"`
	LastNameLower      string            `json:"lastNameLowerCase"`
	Email              string            `json:"email"`
	Phone              string            `json:"phone"`
	Source             string            `json:"source"`
	Country            string            `json:"country"`
	DateAdded          string            `json:"dateAdded"`
	DateUpdated        string            `json:"dateUpdated"`
	Tags               []string          `json:"tags"`
	CustomFields       []customFieldItem `json:"customFields"`
	AttributionSource  *attribution      `json:"attributionSource"`
	LastAttributionSrc *attribution      `json:"lastAttributionSource"`
	SearchAfter        json.RawMessage   `json:"searchAfter"`
}

type customFieldItem struct {
	ID    string          `json:"id"`
	Value json.RawMessage `json:"value"`
}

type attribution struct {
	UTMSource  string `json:"utmSource"`
	UTMMedium  string `json:"utmMedium"`
	UTMContent string `json:"utmContent"`
	UTMTerm    string `json:"utmTerm"`
	UTMKeyword string `json:"utmKeyword"`
	Campaign   string `json:"campaign"`
	CampaignID string `json:"campaignId"`
	AdID       string `json:"adId"`
	GCLID      string `json:"gclid"`
	GBRAID     string `json:"gbraid"`
	WBRAID     string `json:"wbraid"`
	FBCLID     string `json:"fbclid"`
	Referrer   string `json:"referrer"`
	URL        string `json:"url"`
}

// ListContacts implements leads.CRM. cursor is the previous page's last
// searchAfter array as JSON, or empty for the first page. updatedSince
// switches to an incremental read ordered by dateUpdated.
func (c *Client) ListContacts(ctx context.Context, token, locationID, cursor string, updatedSince *time.Time) (leads.ContactsPage, error) {
	req := searchRequest{LocationID: locationID, PageLimit: pageLimit, Sort: []searchSort{{Field: "dateAdded", Direction: "asc"}, {Field: "id", Direction: "asc"}}}
	if updatedSince != nil {
		req.Filters = []searchFilter{{Field: "dateUpdated", Operator: "range", Value: map[string]string{"gte": updatedSince.UTC().Format(time.RFC3339)}}}
		req.Sort = []searchSort{{Field: "dateUpdated", Direction: "asc"}, {Field: "id", Direction: "asc"}}
	}
	if cursor != "" {
		if !json.Valid([]byte(cursor)) {
			return leads.ContactsPage{}, errors.New("GoHighLevel contact cursor is invalid")
		}
		req.SearchAfter = json.RawMessage(cursor)
	}
	body, err := json.Marshal(req)
	if err != nil {
		return leads.ContactsPage{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiBaseURL+"/contacts/search", bytes.NewReader(body))
	if err != nil {
		return leads.ContactsPage{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	var out searchResponse
	if err := c.request(httpReq, &out); err != nil {
		return leads.ContactsPage{}, err
	}
	page := leads.ContactsPage{Contacts: make([]leads.Contact, 0, len(out.Contacts)), Total: &out.Total}
	for _, sc := range out.Contacts {
		page.Contacts = append(page.Contacts, sc.toLead())
	}
	if n := len(out.Contacts); n == pageLimit && len(out.Contacts[n-1].SearchAfter) > 0 {
		page.NextCursor = string(out.Contacts[n-1].SearchAfter)
	}
	return page, nil
}

func (sc searchContact) toLead() leads.Contact {
	first, last := sc.FirstName, sc.LastName
	if first == "" {
		first = sc.FirstNameLower
	}
	if last == "" {
		last = sc.LastNameLower
	}
	c := leads.Contact{
		ID: sc.ID, FirstName: first, LastName: last, Email: sc.Email, Phone: sc.Phone, Source: sc.Source, Country: sc.Country,
		DateAdded: parseTime(sc.DateAdded), DateUpdated: parseTime(sc.DateUpdated), Tags: sc.Tags, CustomFields: map[string]string{},
	}
	for _, f := range sc.CustomFields {
		c.CustomFields[f.ID] = customFieldString(f.Value)
	}
	if sc.AttributionSource != nil {
		c.Attributions = append(c.Attributions, sc.AttributionSource.toLead(true))
	}
	if sc.LastAttributionSrc != nil {
		c.Attributions = append(c.Attributions, sc.LastAttributionSrc.toLead(false))
	}
	raw, _ := json.Marshal(sc)
	c.Raw = raw
	return c
}

func (a attribution) toLead(first bool) leads.Attribution {
	return leads.Attribution{
		IsFirst: first, UTMSource: a.UTMSource, UTMMedium: a.UTMMedium, UTMCampaign: a.Campaign, UTMContent: a.UTMContent,
		UTMTerm: firstNonEmpty(a.UTMTerm, a.UTMKeyword), CampaignID: a.CampaignID, AdID: a.AdID,
		GCLID: a.GCLID, GBRAID: a.GBRAID, WBRAID: a.WBRAID, FBCLID: a.FBCLID, Referrer: a.Referrer, LandingURL: a.URL,
	}
}

// customFieldString flattens a custom field value: strings as they are,
// numbers and booleans formatted, arrays joined with ", ".
func customFieldString(raw json.RawMessage) string {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return strings.Trim(string(raw), `"`)
	}
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	case []any:
		parts := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case nil:
		return ""
	}
	return string(raw)
}

func parseTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
		if t, err := time.Parse(layout, s); err == nil {
			u := t.UTC()
			return &u
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

var _ leads.CRM = (*Client)(nil)
var _ = core.Invalid
