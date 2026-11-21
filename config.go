package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://sentry.io/api/0"
	configKey      = "kanban-sentry"
	configKeyAlt   = "pegg-kanban-sentry"
)

var configKeys = []string{"sentry", "pegg-sentry", configKey, configKeyAlt}

type Settings struct {
	Enabled               bool   `json:"enabled"`
	BaseURL               string `json:"base_url"`
	Token                 string `json:"token"`
	Organization          string `json:"organization"`
	Project               string `json:"project"`
	DefaultQuery          string `json:"default_query"`
	SyncLimit             int    `json:"sync_limit"`
	ChangeRequestProvider string `json:"change_request_provider"`
	Contact               bool   `json:"contact"`
	ResolveOnSuccess      bool   `json:"resolve_on_success"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
	Workdir               string `json:"-"`
}

func defaultSettings() Settings {
	wd, _ := os.Getwd()
	return Settings{
		Enabled:               true,
		BaseURL:               defaultBaseURL,
		DefaultQuery:          "is:unresolved",
		SyncLimit:             25,
		ChangeRequestProvider: "github",
		Contact:               true,
		RequestTimeoutSeconds: 30,
		Workdir:               wd,
	}
}

func (s *Settings) applyHost(configJSON []byte) {
	if len(configJSON) == 0 {
		return
	}
	var host struct {
		Plugins struct {
			Options map[string]json.RawMessage `json:"options"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal(configJSON, &host); err != nil {
		return
	}
	for _, key := range configKeys {
		raw := host.Plugins.Options[key]
		if len(raw) == 0 || string(raw) == "null" {
			continue
		}
		_ = json.Unmarshal(raw, s)
	}
}

func (s *Settings) applyOptions(options json.RawMessage) error {
	if len(options) == 0 || string(options) == "null" {
		return nil
	}
	if err := json.Unmarshal(options, s); err != nil {
		return fmt.Errorf("parse %s options: %w", configKey, err)
	}
	return nil
}

func (s *Settings) applyEnv() {
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_OPTIONS")); v != "" {
		_ = s.applyOptions(json.RawMessage(v))
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_BASE_URL")); v != "" {
		s.BaseURL = v
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_TOKEN")); v != "" {
		s.Token = v
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_ORG")); v != "" {
		s.Organization = v
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_PROJECT")); v != "" {
		s.Project = v
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_QUERY")); v != "" {
		s.DefaultQuery = v
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_SYNC_LIMIT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.SyncLimit = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_CHANGE_REQUEST_PROVIDER")); v != "" {
		s.ChangeRequestProvider = v
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_TIMEOUT_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			s.RequestTimeoutSeconds = n
		}
	}
	if v := strings.TrimSpace(os.Getenv("PEGG_SENTRY_DISABLED")); v == "1" || strings.EqualFold(v, "true") {
		s.Enabled = false
	}
	s.normalize()
}

func (s *Settings) normalize() {
	s.BaseURL = normalizeBaseURL(s.BaseURL)
	s.Token = strings.TrimSpace(s.Token)
	s.Organization = strings.TrimSpace(s.Organization)
	s.Project = strings.TrimSpace(s.Project)
	s.DefaultQuery = strings.TrimSpace(s.DefaultQuery)
	if s.DefaultQuery == "" {
		s.DefaultQuery = "is:unresolved"
	}
	if s.SyncLimit <= 0 {
		s.SyncLimit = 25
	}
	s.ChangeRequestProvider = strings.ToLower(strings.TrimSpace(s.ChangeRequestProvider))
	if s.ChangeRequestProvider == "" {
		s.ChangeRequestProvider = "github"
	}
	if s.RequestTimeoutSeconds <= 0 {
		s.RequestTimeoutSeconds = 30
	}
	if strings.TrimSpace(s.Workdir) == "" {
		s.Workdir, _ = os.Getwd()
	}
	s.Workdir = expandHome(s.Workdir)
}

func normalizeBaseURL(value string) string {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return defaultBaseURL
	}
	value = strings.TrimSuffix(value, "/api/0")
	return value + "/api/0"
}

func (s Settings) timeout() time.Duration {
	if s.RequestTimeoutSeconds <= 0 {
		return 30 * time.Second
	}
	return time.Duration(s.RequestTimeoutSeconds) * time.Second
}

func (s Settings) target() string {
	if s.Organization == "" || s.Project == "" {
		return ""
	}
	return s.Organization + "/" + s.Project
}

func (s Settings) authenticated() bool {
	return s.Token != ""
}

func (s Settings) configured() bool {
	return s.target() != "" && s.authenticated()
}

func (s Settings) secrets() []string {
	if s.Token == "" {
		return nil
	}
	return []string{s.Token}
}

func redactSettings(msg string, s Settings) string {
	for _, secret := range s.secrets() {
		msg = strings.ReplaceAll(msg, secret, "[redacted]")
	}
	return msg
}

func redactError(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s", err.Error())
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		if path == "~" {
			return home
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
