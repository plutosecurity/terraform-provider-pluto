// Package client calls the Pluto policy API.
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

// ManagedBy is written on every policy this provider creates or updates.
const ManagedBy = "terraform"

// StatusDeleting is reported by a policy whose deletion has not finished.
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

// Policy is both the write body and the read shape. Category and EnforcementTool are not sent on update.
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

// A definition is a policy as the dashboard's editor reads and writes it. The provider passes it
// through as raw JSON and reads only the id and version Pluto assigns.

type definitionBody struct {
	Policy    json.RawMessage `json:"policy"`
	ManagedBy string          `json:"managed_by,omitempty"`
	Version   string          `json:"version,omitempty"`
}

// SavedDefinition is a policy as Pluto returns it.
type SavedDefinition struct {
	Policy  json.RawMessage
	ID      string
	Version string
}

const definitions = "/policy-definitions"

func (c *Client) saved(ctx context.Context, method, path string, body any) (*SavedDefinition, error) {
	var out struct {
		Policy json.RawMessage `json:"policy"`
	}
	if err := c.do(ctx, method, path, body, &out); err != nil {
		return nil, err
	}
	var ids struct {
		ID   string `json:"id"`
		Meta struct {
			Version string `json:"version"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(out.Policy, &ids); err != nil {
		return nil, err
	}
	return &SavedDefinition{Policy: out.Policy, ID: ids.ID, Version: ids.Meta.Version}, nil
}

func (c *Client) GetDefinition(ctx context.Context, id string) (*SavedDefinition, error) {
	return c.saved(ctx, http.MethodGet, definitions+"/"+id, nil)
}

func (c *Client) CreateDefinition(ctx context.Context, definition json.RawMessage) (*SavedDefinition, error) {
	return c.saved(ctx, http.MethodPost, definitions, definitionBody{Policy: definition, ManagedBy: ManagedBy})
}

func (c *Client) UpdateDefinition(ctx context.Context, id, version string, definition json.RawMessage) (*SavedDefinition, error) {
	body := definitionBody{Policy: definition, ManagedBy: ManagedBy, Version: version}
	return c.saved(ctx, http.MethodPut, definitions+"/"+id, body)
}

func (c *Client) DeleteDefinition(ctx context.Context, id string) error {
	err := c.do(ctx, http.MethodDelete, definitions+"/"+id, nil, nil)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	return err
}

// NormalizeDefinition is the definition as Pluto reads it back: every defaulted field written out.
func (c *Client) NormalizeDefinition(ctx context.Context, definition json.RawMessage) (json.RawMessage, error) {
	var out struct {
		Policy json.RawMessage `json:"policy"`
	}
	return out.Policy, c.do(ctx, http.MethodPost, definitions+"/normalize", definitionBody{Policy: definition}, &out)
}
