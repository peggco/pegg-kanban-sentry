package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	peggplugin "github.com/peggco/pegg/pkg/plugin"
)

func testPlugin(t *testing.T, handler http.HandlerFunc) *Plugin {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	settings := defaultSettings()
	settings.BaseURL = server.URL
	settings.Token = "sentry-token"
	settings.Organization = "acme"
	settings.Project = "backend"
	p := &Plugin{}
	p.setSettings(settings)
	return p
}

func executeHookT(t *testing.T, p *Plugin, name string, kind peggplugin.HookKind, value any) *peggplugin.HookResult {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.ExecuteHook(peggplugin.HookRequest{Name: name, Kind: kind, Value: raw, TimeoutMS: 5000})
	if err != nil {
		t.Fatal(err)
	}
	if result == nil {
		t.Fatalf("%s returned nil result", name)
	}
	return result
}

func TestHookSpecsAndCapability(t *testing.T) {
	p := &Plugin{}
	specs, err := p.Hooks()
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 4 {
		t.Fatalf("specs = %#v", specs)
	}
	want := map[string]peggplugin.HookKind{
		hookProviders:           peggplugin.HookFilter,
		hookIssueFetch:          peggplugin.HookFilter,
		hookChangeRequestRoutes: peggplugin.HookFilter,
		hookCardSucceeded:       peggplugin.HookAction,
	}
	for _, spec := range specs {
		kind, ok := want[spec.Name]
		if !ok {
			t.Fatalf("unexpected spec %#v", spec)
		}
		if spec.Kind != kind {
			t.Errorf("%s kind = %q, want %q", spec.Name, spec.Kind, kind)
		}
	}
	capabilities := p.Capabilities()
	if len(capabilities) != 1 || capabilities[0] != string(peggplugin.CapabilityHooks) {
		t.Fatalf("capabilities = %#v", capabilities)
	}
}

func TestProvidersHook(t *testing.T) {
	p := testPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("providers hook made an API call: %s", r.URL.Path)
	})
	result := executeHookT(t, p, hookProviders, peggplugin.HookFilter, []ProviderInfo{{ID: "other"}})
	if !result.Handled {
		t.Fatal("providers hook not handled")
	}
	var providers []ProviderInfo
	if err := json.Unmarshal(result.Value, &providers); err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 || providers[1].ID != pluginName {
		t.Fatalf("providers = %#v", providers)
	}
	if providers[1].Target != "acme/backend" || !providers[1].Available {
		t.Fatalf("provider info = %#v", providers[1])
	}
	if providers[1].RepoTarget {
		t.Fatal("sentry must not be a repo target")
	}
}

func TestIssueFetchHook(t *testing.T) {
	p := testPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/acme/backend/issues/" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sentry-token" {
			t.Errorf("auth = %q", got)
		}
		fmt.Fprint(w, `[{"id":"123","shortId":"BACKEND-1","title":"TypeError: boom","culprit":"app/main.py in view","permalink":"https://sentry.io/issues/123/","status":"unresolved","level":"error","count":"42","userCount":7,"firstSeen":"2026-01-01T00:00:00Z","lastSeen":"2026-01-02T00:00:00Z","isUnhandled":true}]`)
	})
	result := executeHookT(t, p, hookIssueFetch, peggplugin.HookFilter, IssueFetch{Provider: pluginName})
	if !result.Handled {
		t.Fatal("issue fetch not handled")
	}
	var payload IssueFetch
	if err := json.Unmarshal(result.Value, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Issues) != 1 {
		t.Fatalf("issues = %#v", payload.Issues)
	}
	issue := payload.Issues[0]
	if issue.Number != 123 || issue.Key != "BACKEND-1" || issue.Title != "TypeError: boom" {
		t.Fatalf("issue = %#v", issue)
	}
	if issue.Priority != "high" || issue.URL == "" {
		t.Fatalf("issue fields = %#v", issue)
	}
}

func TestRoutesHook(t *testing.T) {
	p := &Plugin{settings: Settings{Enabled: true, ChangeRequestProvider: "github"}}
	result := executeHookT(t, p, hookChangeRequestRoutes, peggplugin.HookFilter, map[string]string{})
	if !result.Handled {
		t.Fatal("routes hook not handled")
	}
	var routes map[string]string
	if err := json.Unmarshal(result.Value, &routes); err != nil {
		t.Fatal(err)
	}
	if routes[pluginName] != "github" {
		t.Fatalf("routes = %#v", routes)
	}

	result = executeHookT(t, p, hookChangeRequestRoutes, peggplugin.HookFilter, map[string]string{pluginName: "gitlab"})
	if err := json.Unmarshal(result.Value, &routes); err != nil {
		t.Fatal(err)
	}
	if routes[pluginName] != "gitlab" {
		t.Fatalf("existing route overwritten: %#v", routes)
	}
}

func TestCardSucceededCommentsAndResolves(t *testing.T) {
	var commented, resolved bool
	p := testPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/issues/123/comments/":
			commented = true
			fmt.Fprint(w, `{"id":"c1"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/issues/123/":
			resolved = true
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["status"] != "resolved" {
				t.Errorf("resolve body = %#v", body)
			}
			fmt.Fprint(w, `{"id":"123","status":"resolved"}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	})
	settings := p.currentSettings()
	settings.ResolveOnSuccess = true
	p.setSettings(settings)

	result := executeHookT(t, p, hookCardSucceeded, peggplugin.HookAction, CardSucceeded{
		Card: Card{
			ID:    "card-1",
			Title: "Fix boom",
			Issue: &IssueRef{Provider: pluginName, Repo: "acme/backend", Number: 123, Key: "BACKEND-1"},
		},
		Summary:  "Fixed the null check",
		Feedback: true,
	})
	if !result.Handled {
		t.Fatal("card hook not handled")
	}
	if !commented || !resolved {
		t.Fatalf("commented = %t, resolved = %t", commented, resolved)
	}
}

func TestCardSucceededOtherProviderNoop(t *testing.T) {
	p := testPlugin(t, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
	})
	result := executeHookT(t, p, hookCardSucceeded, peggplugin.HookAction, CardSucceeded{
		Card: Card{Issue: &IssueRef{Provider: "github", Number: 1}},
	})
	if result.Handled {
		t.Fatal("card hook should not be handled for another provider")
	}
}
