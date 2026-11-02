package provider

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// fakeAPI mimics /api/ext/v1/policies: omitted PATCH fields are left alone, category and
// enforcement_tool cannot be sent on update, and a deleted policy reads as "deleting".
type fakeAPI struct {
	mu       sync.Mutex
	policies map[string]map[string]any
	nextID   int
}

func newFakeAPI(t *testing.T) (*fakeAPI, *httptest.Server) {
	f := &fakeAPI{policies: map[string]map[string]any{}}
	srv := httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeAPI) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer pluto_test" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/ext/v1/policies/")
	var body map[string]any
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body) //nolint:errcheck // GET and DELETE carry no body
	}
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/api/ext/v1/policies":
		f.nextID++
		id = fmt.Sprintf("p%d", f.nextID)
		body["id"], body["status"] = id, "active"
		if _, ok := body["targeted_scopes"]; !ok || body["targeted_scopes"] == nil {
			body["targeted_scopes"] = []any{}
		}
		f.policies[id] = body
	case f.policies[id] == nil:
		http.NotFound(w, r)
		return
	case r.Method == http.MethodPatch:
		if _, ok := body["category"]; ok {
			http.Error(w, "category cannot be changed", http.StatusBadRequest)
			return
		}
		if _, ok := body["enforcement_tool"]; ok {
			http.Error(w, "enforcement integration cannot be changed", http.StatusBadRequest)
			return
		}
		for k, v := range body {
			f.policies[id][k] = v
		}
	case r.Method == http.MethodDelete:
		f.policies[id]["status"] = "deleting"
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true}) //nolint:errcheck // test server
		return
	}
	_ = json.NewEncoder(w).Encode(f.policies[id]) //nolint:errcheck // test server
}

func (f *fakeAPI) policy(id string) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.policies[id]
}

func providers() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"pluto": providerserver.NewProtocol6WithError(New("test")()),
	}
}

func config(host, name, tools string) string {
	return fmt.Sprintf(`
provider "pluto" {
  host    = %q
  api_key = "pluto_test"
}

resource "pluto_policy" "notetakers" {
  name             = %q
  category         = "AI Notetakers"
  policy_type      = "block"
  enforcement_tool = "Falcon"
  tools            = [%s]
  targeted_scopes = [{
    type   = "user_department"
    values = ["Engineering"]
  }]
}
`, host, name, tools)
}

func TestPolicyLifecycle(t *testing.T) {
	api, srv := newFakeAPI(t)
	const addr = "pluto_policy.notetakers"
	var id string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providers(),
		Steps: []resource.TestStep{
			{
				Config: config(srv.URL, "Block notetakers", `"Otter", "Fireflies"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "tools.#", "2"),
					resource.TestCheckResourceAttr(addr, "enabled", "true"),
					resource.TestCheckResourceAttr(addr, "targeted_scopes.0.values.0", "Engineering"),
					func(s *terraform.State) error {
						id = s.RootModule().Resources[addr].Primary.ID
						if got := api.policy(id)["managed_by"]; got != "terraform" {
							return fmt.Errorf("managed_by = %v, want terraform", got)
						}
						return nil
					},
				),
			},
			{
				Config: config(srv.URL, "Block AI notetakers", `"Otter"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(addr, "name", "Block AI notetakers"),
					resource.TestCheckResourceAttr(addr, "tools.#", "1"),
				),
			},
			{
				ResourceName:      addr,
				ImportState:       true,
				ImportStateVerify: true,
			},
			{
				// A dashboard edit is drift: the next apply puts the configured value back.
				PreConfig: func() { api.policy(id)["name"] = "Edited in the dashboard" },
				Config:    config(srv.URL, "Block AI notetakers", `"Otter"`),
				Check: func(*terraform.State) error {
					if got := api.policy(id)["name"]; got != "Block AI notetakers" {
						return fmt.Errorf("name after apply = %v, want the configured name", got)
					}
					return nil
				},
			},
		},
		CheckDestroy: func(*terraform.State) error {
			if got := api.policy(id)["status"]; got != "deleting" {
				return fmt.Errorf("status after destroy = %v, want deleting", got)
			}
			return nil
		},
	})
}

func TestOmittedToolsAreLeftAlone(t *testing.T) {
	api, srv := newFakeAPI(t)
	cfg := fmt.Sprintf(`
provider "pluto" {
  host    = %q
  api_key = "pluto_test"
}

resource "pluto_policy" "monitor" {
  name        = "Watch notetakers"
  category         = "AI Notetakers"
  policy_type      = "detect"
  enforcement_tool = "Monitor"
}
`, srv.URL)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: providers(),
		Steps: []resource.TestStep{{
			Config: cfg,
			Check: func(s *terraform.State) error {
				id := s.RootModule().Resources["pluto_policy.monitor"].Primary.ID
				if _, sent := api.policy(id)["selected_tools"]; sent {
					return fmt.Errorf("selected_tools was sent although tools is not configured")
				}
				return nil
			},
		}},
	})
}
