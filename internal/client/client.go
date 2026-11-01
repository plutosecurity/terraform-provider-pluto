// Package client calls Pluto's external policy API (/api/ext/v1/policies).
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ManagedBy marks every policy this provider writes, so the dashboard warns that edits are reverted.
const ManagedBy = "terraform"

// StatusDeleting is reported by a policy whose delete was accepted but whose rules are still being torn down.
const StatusDeleting = "deleting"

var ErrNotFound = errors.New("policy not found")

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(host, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(host, "/") + "/api/ext/v1",
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

type Selector struct {
	Type   string   `json:"type"`
	Values []string `json:"values"`
}

type Tool struct {
	Type        string `json:"type"`
	DisplayName string `json:"display_name"`
}

// Policy is both the write body and the read shape. Category and EnforcementTool are omitted on
// update because the API refuses to change them on an existing policy.
type Policy struct {
	ID              string     `json:"id,omitempty"`
	Name            string     `json:"name"`
	Description     *string    `json:"description"`
	Category        string     `json:"category,omitempty"`
	PolicyType      string     `json:"policy_type"`
	EnforcementTool *string    `json:"enforcement_tool,omitempty"`
	Enabled         bool       `json:"enabled"`
	CustomMessage   *string    `json:"custom_message"`
	NotifyEndUser   bool       `json:"notify_end_user"`
	TargetedScopes  []Selector `json:"targeted_scopes"`
	SelectedTools   *[]Tool    `json:"selected_tools,omitempty"`
	ManagedBy       *string    `json:"managed_by"`
	Status          string     `json:"status,omitempty"`
}

func (c *Client) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	var p Policy
	return &p, c.do(ctx, http.MethodGet, "/policies/"+id, nil, &p)
}

func (c *Client) CreatePolicy(ctx context.Context, p Policy) (*Policy, error) {
	var out Policy
	return &out, c.do(ctx, http.MethodPost, "/policies", stamp(p), &out)
}

func (c *Client) UpdatePolicy(ctx context.Context, id string, p Policy) (*Policy, error) {
	p.Category, p.EnforcementTool = "", nil
	var out Policy
	return &out, c.do(ctx, http.MethodPatch, "/policies/"+id, stamp(p), &out)
}

func (c *Client) DeletePolicy(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, "/policies/"+id, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

func stamp(p Policy) Policy {
	managedBy := ManagedBy
	p.ManagedBy = &managedBy
	p.ID, p.Status = "", ""
	return p
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // draining response body
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}
