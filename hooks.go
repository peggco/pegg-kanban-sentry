package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	peggplugin "github.com/peggco/pegg/pkg/plugin"
)

const (
	hookProviders           = "kanban.providers"
	hookIssueFetch          = "kanban.issues.fetch"
	hookChangeRequestRoutes = "kanban.change_request.routes"
	hookCardSucceeded       = "kanban.card.succeeded"

	hookPriority = 10
)

type ProviderInfo struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Enabled       bool   `json:"enabled"`
	Available     bool   `json:"available"`
	Authenticated bool   `json:"authenticated"`
	Detail        string `json:"detail,omitempty"`
	Target        string `json:"target,omitempty"`
	SyncMinutes   int    `json:"sync_minutes,omitempty"`
	RepoTarget    bool   `json:"repo_target,omitempty"`
}

type HookIssue struct {
	ExternalID string   `json:"external_id,omitempty"`
	Number     int      `json:"number,omitempty"`
	Key        string   `json:"key,omitempty"`
	Title      string   `json:"title"`
	Body       string   `json:"body,omitempty"`
	URL        string   `json:"url,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	Assignees  []string `json:"assignees,omitempty"`
	State      string   `json:"state,omitempty"`
	Priority   string   `json:"priority,omitempty"`
	CreatedAt  string   `json:"created_at,omitempty"`
	UpdatedAt  string   `json:"updated_at,omitempty"`
}

type IssueFetch struct {
	Provider string      `json:"provider"`
	Target   string      `json:"target,omitempty"`
	Labels   []string    `json:"labels,omitempty"`
	State    string      `json:"state,omitempty"`
	Limit    int         `json:"limit,omitempty"`
	Query    string      `json:"query,omitempty"`
	Issues   []HookIssue `json:"issues"`
}

type IssueRef struct {
	Provider string `json:"provider,omitempty"`
	Repo     string `json:"repo"`
	Number   int    `json:"number"`
	Key      string `json:"key,omitempty"`
	URL      string `json:"url"`
}

type Card struct {
	ID    string    `json:"id"`
	Title string    `json:"title"`
	Body  string    `json:"body,omitempty"`
	Issue *IssueRef `json:"issue,omitempty"`
}

type CardSucceeded struct {
	Card                  Card   `json:"card"`
	Summary               string `json:"summary,omitempty"`
	Feedback              bool   `json:"feedback"`
	CreateChangeRequest   bool   `json:"create_change_request"`
	ChangeRequestProvider string `json:"change_request_provider,omitempty"`
}

type cardSucceededResult struct {
	Key       string `json:"key,omitempty"`
	Commented bool   `json:"commented"`
	Resolved  bool   `json:"resolved"`
}

func hookSpecs() []peggplugin.HookSpec {
	return []peggplugin.HookSpec{
		{
			Name:        hookProviders,
			Kind:        peggplugin.HookFilter,
			Priority:    hookPriority,
			Description: "Report Sentry as a Kanban provider",
		},
		{
			Name:        hookIssueFetch,
			Kind:        peggplugin.HookFilter,
			Priority:    hookPriority,
			Description: "Fetch Sentry issues for Kanban import",
		},
		{
			Name:        hookChangeRequestRoutes,
			Kind:        peggplugin.HookFilter,
			Priority:    hookPriority,
			Description: "Route Sentry issues to the default change-request provider",
		},
		{
			Name:        hookCardSucceeded,
			Kind:        peggplugin.HookAction,
			Priority:    hookPriority,
			Description: "Comment on and resolve the Sentry issue when a Kanban card succeeds",
		},
	}
}

func (p *Plugin) Hooks() ([]peggplugin.HookSpec, error) {
	return hookSpecs(), nil
}

func (p *Plugin) ExecuteHook(req peggplugin.HookRequest) (*peggplugin.HookResult, error) {
	settings := p.currentSettings()
	if !settings.Enabled {
		return &peggplugin.HookResult{Handled: false}, nil
	}
	timeout := settings.timeout()
	if req.TimeoutMS > 0 {
		timeout = time.Duration(req.TimeoutMS) * time.Millisecond
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	value, handled, err := p.executeHook(ctx, settings, req)
	if err != nil {
		return nil, fmt.Errorf("%s", redactSettings(err.Error(), settings))
	}
	return &peggplugin.HookResult{Value: value, Handled: handled}, nil
}

func (p *Plugin) executeHook(ctx context.Context, settings Settings, req peggplugin.HookRequest) (json.RawMessage, bool, error) {
	switch req.Name {
	case hookProviders:
		return p.providersHook(settings, req)
	case hookIssueFetch:
		var payload IssueFetch
		if err := decodeHookValue(req.Value, &payload); err != nil {
			return nil, false, err
		}
		if !strings.EqualFold(strings.TrimSpace(payload.Provider), pluginName) {
			return nil, false, nil
		}
		return p.fetchIssuesHook(ctx, settings, payload)
	case hookChangeRequestRoutes:
		return p.routesHook(settings, req)
	case hookCardSucceeded:
		var payload CardSucceeded
		if err := decodeHookValue(req.Value, &payload); err != nil {
			return nil, false, err
		}
		if payload.Card.Issue == nil || !strings.EqualFold(strings.TrimSpace(payload.Card.Issue.Provider), pluginName) {
			return nil, false, nil
		}
		return p.cardSucceededHook(ctx, settings, payload)
	default:
		return nil, false, nil
	}
}

func decodeHookValue(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode %s payload: %w", pluginName, err)
	}
	return nil
}

func (p *Plugin) providersHook(settings Settings, req peggplugin.HookRequest) (json.RawMessage, bool, error) {
	var providers []ProviderInfo
	if err := decodeHookValue(req.Value, &providers); err != nil {
		return nil, false, err
	}
	if !settings.Enabled {
		return nil, false, nil
	}
	info := providerInfo(settings)
	replaced := false
	for i := range providers {
		if providers[i].ID == pluginName {
			providers[i] = info
			replaced = true
			break
		}
	}
	if !replaced {
		providers = append(providers, info)
	}
	value, err := json.Marshal(providers)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func providerInfo(settings Settings) ProviderInfo {
	authenticated := settings.authenticated()
	target := settings.target()
	available := settings.Enabled && authenticated && target != ""
	return ProviderInfo{
		ID:            pluginName,
		Label:         pluginDisplay,
		Enabled:       settings.Enabled,
		Available:     available,
		Authenticated: authenticated,
		Detail:        providerDetail(settings, authenticated, target),
		Target:        target,
	}
}

func providerDetail(settings Settings, authenticated bool, target string) string {
	switch {
	case !settings.Enabled:
		return "Plugin disabled; enable Sentry to sync issues"
	case !authenticated:
		return "Set a Sentry auth token to sync issues"
	case target == "":
		return "Set organization and project to select issues"
	default:
		return "Ready to sync from " + target
	}
}

func (p *Plugin) fetchIssuesHook(ctx context.Context, settings Settings, payload IssueFetch) (json.RawMessage, bool, error) {
	org, project := splitTarget(payload.Target)
	if org == "" {
		org = settings.Organization
	}
	if project == "" {
		project = settings.Project
	}
	if org == "" || project == "" {
		return nil, false, nil
	}
	query := strings.TrimSpace(payload.Query)
	if query == "" {
		query = settings.DefaultQuery
	}
	limit := payload.Limit
	if limit <= 0 {
		limit = settings.SyncLimit
	}
	issues, err := newClient(settings).listIssues(ctx, org, project, query, limit)
	if err != nil {
		return nil, false, err
	}
	for _, issue := range issues {
		payload.Issues = append(payload.Issues, hookIssue(issue))
	}
	if payload.Issues == nil {
		payload.Issues = []HookIssue{}
	}
	payload.Target = org + "/" + project
	value, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func splitTarget(target string) (string, string) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", ""
	}
	parts := strings.SplitN(target, "/", 2)
	if len(parts) != 2 {
		return target, ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func (p *Plugin) routesHook(settings Settings, req peggplugin.HookRequest) (json.RawMessage, bool, error) {
	routes := map[string]string{}
	if len(req.Value) > 0 && string(req.Value) != "null" {
		if err := json.Unmarshal(req.Value, &routes); err != nil {
			return nil, false, fmt.Errorf("decode %s payload: %w", hookChangeRequestRoutes, err)
		}
	}
	if routes == nil {
		routes = map[string]string{}
	}
	provider := strings.ToLower(strings.TrimSpace(settings.ChangeRequestProvider))
	if provider == "" {
		provider = "github"
	}
	if _, exists := routes[pluginName]; !exists {
		routes[pluginName] = provider
	}
	value, err := json.Marshal(routes)
	if err != nil {
		return nil, false, fmt.Errorf("encode %s payload: %w", hookChangeRequestRoutes, err)
	}
	return value, true, nil
}

func (p *Plugin) cardSucceededHook(ctx context.Context, settings Settings, payload CardSucceeded) (json.RawMessage, bool, error) {
	issue := payload.Card.Issue
	if issue == nil || issue.Number <= 0 {
		return nil, false, nil
	}
	client := newClient(settings)
	result := cardSucceededResult{Key: issue.Key}
	if settings.Contact && payload.Feedback && strings.TrimSpace(payload.Summary) != "" {
		if err := client.commentIssue(ctx, fmt.Sprintf("%d", issue.Number), payload.Summary); err != nil {
			return nil, false, err
		}
		result.Commented = true
	}
	if settings.ResolveOnSuccess {
		if err := client.resolveIssue(ctx, fmt.Sprintf("%d", issue.Number)); err != nil {
			return nil, false, err
		}
		result.Resolved = true
	}
	value, err := json.Marshal(result)
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}
