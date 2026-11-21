package main

import (
	"fmt"
	"sync"

	peggplugin "github.com/peggco/pegg/pkg/plugin"
)

const (
	version       = "1.0.0"
	pluginID      = "kanban-sentry"
	pluginName    = "sentry"
	pluginDisplay = "Sentry"
)

type Plugin struct {
	peggplugin.Base

	mu       sync.Mutex
	settings Settings
}

func (p *Plugin) Name() string { return pluginID }

func (p *Plugin) Version() string { return version }

func (p *Plugin) Description() string {
	return "Kanban integration for Sentry issues"
}

func (p *Plugin) Boot(deps peggplugin.Deps) error {
	settings := defaultSettings()
	if deps.Workdir != "" {
		settings.Workdir = deps.Workdir
	}
	settings.applyHost(deps.ConfigJSON)
	if err := settings.applyOptions(deps.Options); err != nil {
		return fmt.Errorf("parse %s options: %w", pluginID, err)
	}
	settings.applyEnv()
	settings.normalize()

	p.mu.Lock()
	p.settings = settings
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Capabilities() []string {
	return []string{string(peggplugin.CapabilityHooks)}
}

func (p *Plugin) currentSettings() Settings {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.settings
}

func (p *Plugin) setSettings(settings Settings) {
	p.mu.Lock()
	p.settings = settings
	p.mu.Unlock()
}
