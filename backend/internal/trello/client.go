// Package trello is a minimal client for the parts of the Trello REST API
// this project needs (creating cards). It authenticates with a single
// API key + token pair — no OAuth, no per-user authentication.
package trello

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.trello.com/1"

// maxErrorBodyBytes caps how much of a non-2xx response body gets read into
// an error message, to avoid unbounded memory use on a misbehaving server.
const maxErrorBodyBytes = 4096

// Client creates Trello cards using a single API key + token pair (the
// account-level "power-up" style credentials from trello.com/app-key), not
// per-user OAuth.
type Client struct {
	apiKey     string
	token      string
	httpClient *http.Client
	// baseURL is overridden in tests to point at an httptest.Server.
	baseURL string
}

// NewClient builds a Client. apiKey and token are never logged or included
// in returned errors.
func NewClient(apiKey, token string) *Client {
	return &Client{
		apiKey:     apiKey,
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    defaultBaseURL,
	}
}

// CreateCardInput describes a card to create.
type CreateCardInput struct {
	// ListID is the Trello list to create the card in.
	ListID string
	// Name is the card's title.
	Name string
	// Description is the card's body text (Trello's "desc" field).
	Description string
	// LabelIDs are the Trello label IDs to attach to the card, if any.
	LabelIDs []string
}

// CreatedCard is the subset of Trello's card-creation response this project uses.
type CreatedCard struct {
	// ID is the created card's Trello ID.
	ID string
	// URL is the created card's full Trello URL.
	URL string
	// ShortURL is the created card's short Trello URL.
	ShortURL string
}

// cardResponse mirrors the fields of Trello's POST /1/cards response we care about.
type cardResponse struct {
	ID       string `json:"id"`
	URL      string `json:"url"`
	ShortURL string `json:"shortUrl"`
}

// CreateCard creates a Trello card via POST /1/cards. Credentials are sent in
// the request body (not the URL) so they don't end up in access logs.
func (c *Client) CreateCard(ctx context.Context, input CreateCardInput) (*CreatedCard, error) {
	if input.ListID == "" {
		return nil, fmt.Errorf("trello: list ID is required")
	}
	if input.Name == "" {
		return nil, fmt.Errorf("trello: card name is required")
	}

	form := url.Values{}
	form.Set("key", c.apiKey)
	form.Set("token", c.token)
	form.Set("idList", input.ListID)
	form.Set("name", input.Name)
	if input.Description != "" {
		form.Set("desc", input.Description)
	}
	if len(input.LabelIDs) > 0 {
		form.Set("idLabels", strings.Join(input.LabelIDs, ","))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/cards", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, fmt.Errorf("trello: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("trello: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
	if err != nil {
		return nil, fmt.Errorf("trello: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("trello: create card failed with status %d: %s", resp.StatusCode, string(body))
	}

	var out cardResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("trello: parse response: %w", err)
	}

	return &CreatedCard{ID: out.ID, URL: out.URL, ShortURL: out.ShortURL}, nil
}
