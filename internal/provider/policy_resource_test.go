package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeDefinitions stands in for /api/ext/v1/policy-definitions. Its normalize writes out one
// default, description, so a definition that leaves it out reads back different but equivalent.
type fakeDefinitions struct {
	mu        sync.Mutex
	policies  map[string]map[string]any
	managedBy map[string]any
	next      int
}

func newFakeDefinitions(t *testing.T) (*fakeDefinitions, *httptest.Server) {
	f := &fakeDefinitions{policies: map[string]map[string]any{}, managedBy: map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeDefinitions) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer pluto_test" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck // GET and DELETE carry no body
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/ext/v1/policy-definitions"), "/")
	reply := func(policy map[string]any) {
		_ = json.NewEncoder(w).Encode(map[string]any{"policy": policy}) //nolint:errcheck // test server
	}
	switch {
	case r.Method == http.MethodPost && id == "normalize":
		policy := body["policy"].(map[string]any)
		delete(policy, "id")
		delete(policy, "meta")
		delete(policy, "state")
		if _, ok := policy["description"]; !ok {
			policy["description"] = nil
		}
		reply(policy)
	case r.Method == http.MethodPost && id == "":
		id = fmt.Sprintf("p%d", f.next+1)
		f.store(id, body)
		reply(f.policies[id])
	case f.policies[id] == nil:
		http.NotFound(w, r)
	case r.Method == http.MethodGet:
		reply(f.policies[id])
	case r.Method == http.MethodPut:
		version := f.policies[id]["meta"].(map[string]any)["version"]
		if body["version"] != version {
			http.Error(w, "the policy changed since it was read", http.StatusConflict)
			return
		}
		f.store(id, body)
		reply(f.policies[id])
	case r.Method == http.MethodDelete:
		delete(f.policies, id)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true}) //nolint:errcheck // test server
	}
}

func (f *fakeDefinitions) store(id string, body map[string]any) {
	f.next++
	policy := body["policy"].(map[string]any)
	policy["id"], policy["meta"], policy["state"] = id, map[string]any{"version": fmt.Sprint(f.next)}, map[string]any{"status": "active"}
	f.policies[id] = policy
	if managedBy, sent := body["managed_by"]; sent {
		f.managedBy[id] = managedBy
	}
}

func (f *fakeDefinitions) policy(id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.policies[id]
}

func definitionConfig(host, name, extra string) string {
	return fmt.Sprintf(`
provider "pluto" {
  host    = %q
  api_key = "pluto_test"
}
%s
resource "pluto_policy" "example" {
  definition = jsonencode({ type = "supply-chain", name = %q, enabled = true })
}
`, host, extra, name)
}

func TestPolicyDefinitionLifecycle(t *testing.T) {
	api, srv := newFakeDefinitions(t)
	const addr = "pluto_policy.example"
	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providers(),
		Steps: []resource.TestStep{
			{
				Config: definitionConfig(srv.URL, "Example policy", ""),
				Check: func(s *terraform.State) error {
					id = s.RootModule().Resources[addr].Primary.ID
					if got := api.managedBy[id]; got != "terraform" {
						return fmt.Errorf("managed_by = %v, want terraform", got)
					}
					return nil
				},
			},
			{
				Config: definitionConfig(srv.URL, "Renamed policy", ""),
				Check: func(*terraform.State) error {
					if got := api.policy(id)["name"]; got != "Renamed policy" {
						return fmt.Errorf("name = %v, want the configured name", got)
					}
					return nil
				},
			},
			{
				// An edit outside Terraform is drift: the next apply puts the configured definition back.
				PreConfig: func() { api.policy(id)["enabled"] = false },
				Config:    definitionConfig(srv.URL, "Renamed policy", ""),
				Check: func(*terraform.State) error {
					if got := api.policy(id)["enabled"]; got != true {
						return fmt.Errorf("enabled after apply = %v, want true", got)
					}
					return nil
				},
			},
		},
		CheckDestroy: func(*terraform.State) error {
			if api.policy(id) != nil {
				return fmt.Errorf("policy %s still exists after destroy", id)
			}
			return nil
		},
	})
}

func TestAnImportedDefinitionPlansNothing(t *testing.T) {
	api, srv := newFakeDefinitions(t)
	api.store("p1", map[string]any{"policy": map[string]any{
		"type": "supply-chain", "name": "Example policy", "enabled": true, "description": nil,
	}})
	importBlock := `
import {
  to = pluto_policy.example
  id = "p1"
}
`
	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providers(),
		Steps:                    []resource.TestStep{{Config: definitionConfig(srv.URL, "Example policy", importBlock)}},
	})
}

// legacyConfig is a 0.1 policy; resource is the type it is declared as.
func legacyConfig(host, resourceType, extra string) string {
	return fmt.Sprintf(`
provider "pluto" {
  host    = %q
  api_key = "pluto_test"
}
%s
resource %q "example" {
  name             = "Example policy"
  category         = "Example category"
  policy_type      = "detect"
  enforcement_tool = "Example tool"
}
`, host, extra, resourceType)
}

func TestA01PolicyIsRefusedUntilItMovesToTheLegacyResource(t *testing.T) {
	_, srv := newFakeAPI(t)
	moved := `
moved {
  from = pluto_policy.example
  to   = pluto_policy_legacy.example
}
`
	resource.Test(t, resource.TestCase{
		Steps: []resource.TestStep{
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"pluto": {Source: "plutosecurity/pluto", VersionConstraint: "0.1.1"},
				},
				Config: legacyConfig(srv.URL, "pluto_policy", ""),
			},
			{
				ProtoV6ProviderFactories: providers(),
				Config:                   definitionConfig(srv.URL, "Example policy", ""),
				ExpectError:              regexp.MustCompile(`pluto_policy_legacy`),
			},
			{ProtoV6ProviderFactories: providers(), Config: legacyConfig(srv.URL, "pluto_policy_legacy", moved)},
		},
	})
}
