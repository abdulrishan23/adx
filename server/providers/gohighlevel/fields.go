package gohighlevel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
)

// Name identifies the field writer in the outreach configuration view.
func (c *Client) Name() string { return "gohighlevel" }

// SetContactFields writes custom field values on one contact with
// PUT /contacts/{id} (API version 2021-07-28). Only the given fields are
// sent; nothing else on the contact is touched. A field id that looks like
// a standard contact field (no underscore prefix convention is assumed) is
// sent as a custom field; GoHighLevel rejects unknown ids with a 400 which
// surfaces as a plain error, never as a silent success.
func (c *Client) SetContactFields(ctx context.Context, token, contactID string, fields map[string]string) error {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return errors.New("contact id is required")
	}
	if len(fields) == 0 {
		return errors.New("no fields to set")
	}
	type item struct {
		ID    string `json:"id"`
		Value string `json:"field_value"`
	}
	items := make([]item, 0, len(fields))
	for id, v := range fields {
		items = append(items, item{ID: id, Value: v})
	}
	body, err := json.Marshal(map[string]any{"customFields": items})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.apiBaseURL+"/contacts/"+url.PathEscape(contactID), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	var out json.RawMessage
	return c.request(req, &out)
}
