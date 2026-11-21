<div align="center">

<img src="https://cdn.pegg.dev/logo/infinity.gif" alt="Pegg" width="420" />

# Pegg Kanban Sentry

### Sentry issues on your Kanban board.

A Kanban integration for Sentry. It imports issues and routes them to a git host so a pull request is opened when a card succeeds.

[Plugins docs](https://docs.pegg.dev/pegg/configurations/plugins/) · [Releases](https://github.com/peggco/pegg-kanban-sentry/releases) · [Website](https://pegg.dev) · [Discussions](https://github.com/peggco/pegg/discussions)

[![Latest release](https://img.shields.io/github/v/release/peggco/pegg-kanban-sentry?style=flat-square&sort=semver)](https://github.com/peggco/pegg-kanban-sentry/releases)
[![Go 1.26.5+](https://img.shields.io/badge/go-1.26.5%2B-00ADD8?style=flat-square&logo=go&logoColor=white)](https://github.com/peggco/pegg-kanban-sentry)
[![Plugin protocol v2](https://img.shields.io/badge/plugin%20protocol-v2-black?style=flat-square)](https://docs.pegg.dev/pegg/configurations/plugins/)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-black?style=flat-square)](https://github.com/peggco/pegg-kanban-sentry/blob/HEAD/LICENSE)

**Install**

```bash
pegg plugins install peggco/pegg-kanban
pegg plugins install peggco/pegg-kanban-sentry
```

Hooks-only. Install `pegg-kanban` alongside this plugin; it contributes no agent tools.
</div>

## Setup

The plugin talks to the Sentry REST API with a bearer token.

1. Create a Sentry auth token with `project:read` (and `event:write` for comments,
   `event:admin` to resolve) at <https://sentry.io/settings/account/api/auth-tokens/>.
2. Add the plugin options to `~/.pegg/pegg.json`:

```json
{
  "plugins": {
    "options": {
      "kanban-sentry": {
        "enabled": true,
        "base_url": "https://sentry.io/api/0",
        "token": "sntrys_...",
        "organization": "acme",
        "project": "backend",
        "default_query": "is:unresolved",
        "sync_limit": 25,
        "change_request_provider": "github",
        "contact": true,
        "resolve_on_success": false,
        "request_timeout_seconds": 30
      }
    }
  }
}
```

For self-hosted Sentry set `base_url` to `https://sentry.example.com/api/0`.

The preferred option keys are `kanban-sentry` and `pegg-kanban-sentry`. The legacy
`sentry` and `pegg-sentry` keys are still read as fallbacks.

| Setting | Meaning |
|---|---|
| `token` | Sentry auth token, sent as `Authorization: Bearer`. |
| `organization` | Organization slug. |
| `project` | Project slug. |
| `default_query` | Sentry issue search query. Default `is:unresolved`. |
| `sync_limit` | Maximum issues per sync. Default `25`. |
| `change_request_provider` | Change-request target for Sentry issues. Default `github`. |
| `contact` | Post the run summary as a Sentry comment. Default `true`. |
| `resolve_on_success` | Mark the Sentry issue resolved after a successful run. |

Environment overrides:

| Variable | Purpose |
|---|---|
| `PEGG_SENTRY_OPTIONS` | JSON options object. |
| `PEGG_SENTRY_TOKEN` | Sentry auth token. |
| `PEGG_SENTRY_BASE_URL` | Sentry API base URL. |
| `PEGG_SENTRY_ORG` | Organization slug. |
| `PEGG_SENTRY_PROJECT` | Project slug. |
| `PEGG_SENTRY_QUERY` | Default issue query. |
| `PEGG_SENTRY_SYNC_LIMIT` | Maximum issues per sync. |
| `PEGG_SENTRY_CHANGE_REQUEST_PROVIDER` | Default change-request target. |
| `PEGG_SENTRY_TIMEOUT_SECONDS` | API timeout. |
| `PEGG_SENTRY_DISABLED` | `1` or `true` disables the hooks. |

## Hooks

Provider id: `sentry`.

| Hook | Kind | Priority | Purpose |
|---|---|---|---|
| `kanban.providers` | filter | 10 | Advertise Sentry as an issue provider. |
| `kanban.issues.fetch` | filter | 10 | Fetch Sentry issues for Kanban sync. |
| `kanban.change_request.routes` | filter | 10 | Route Sentry issues to the default change-request provider. |
| `kanban.card.succeeded` | action | 10 | Comment on and resolve the Sentry issue when a card succeeds. |

A disabled plugin, an unknown hook, or a request for another provider returns
`handled: false` without an error.

### Issue sync

`kanban.issues.fetch` accepts `{provider,target,labels,state,limit,query,issues}`.
Only `provider: "sentry"` is handled. `target` is `organization/project`; when
absent it falls back to the configured `organization` and `project`.

Issues are appended with `external_id`, `number` (internal id), `key` (short id),
`title`, `body` (culprit, level and event count), `url` (permalink), `labels`
(level and `unhandled`), `state`, `priority`, and timestamps. The fetch limit is
passed to Sentry as `limit`.

### `kanban.card.succeeded`

Handles cards whose `card.issue.provider` is `sentry`. The issue's internal id is
`card.issue.number`.

- With `contact` and `feedback`, the run summary is posted as a Sentry comment.
- With `resolve_on_success`, the issue is marked resolved.
- The result is `{key,commented,resolved}`.

### Change-request routing

Sentry is an issue source, not a git host. This plugin contributes the default
route `{"sentry": "<change_request_provider>"}` (default `github`), so a resolved
Sentry issue opens a GitHub pull request. The user routing map always wins over
the plugin default:

```json
{
  "plugins": {
    "options": {
      "kanban": {
        "change_request_providers": {"sentry": "gitlab"}
      }
    }
  }
}
```

## Behavior

- `kanban.issues.fetch` maps Sentry issues to cards with `source=sentry`, the
  short id, title, culprit/level/events summary, permalink, labels and priority.
- `kanban.run.prepare` and the pull request are handled by the resolved
  change-request provider (for example `kanban-github`).
- `kanban.card.succeeded` comments the run summary and optionally resolves the
  Sentry issue.
- Tokens are never printed or returned; errors are redacted.

## Build from source

```sh
go vet ./...
go test ./...
go build -trimpath -ldflags "-s -w" -o bin/pegg-kanban-sentry .
install -m 0755 bin/pegg-kanban-sentry "$HOME/.pegg/plugins/pegg-kanban-sentry"
pegg plugins list
```

## Token scopes

- `project:read` is enough for issue sync.
- Add `event:write` to comment and `event:admin` to resolve issues.
- Tokens are never printed; hook errors are redacted before they return to the host.

## License

Apache-2.0. See [LICENSE](LICENSE).
