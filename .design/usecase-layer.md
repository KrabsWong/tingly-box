# internal/usecase

Application-logic layer that sits between config storage (`internal/server/config`)
and every caller surface (CLI, TUI, Web handler, future GUI). Collapses the
request assembly, validation, and error semantics that today are hand-duplicated
per surface into one implementation per domain.

Introduced 2026-08-09. Spec: `.sdlc/docs/refactor-cli-usecase-20260809.spec.md`.

## Why

Rule/Provider/Agent operations exist today as three separate implementations:
CLI (`internal/command/config_rule.go` etc.), TUI (`internal/command/tui/rule_mode.go`
etc.), and the Web handler (`internal/server/module/rule` etc.). They drift —
e.g. the agent routing-key table was hand-copied into two files with a "keep in
sync" comment that had already gone stale. `internal/usecase` is the single
place that logic lives; each caller becomes a thin adapter that does I/O
(prompting, printing, HTTP request/response) and nothing else.

## Contract

**Allowed dependencies**: `internal/server/config`, `internal/typ`,
`internal/loadbalance`, `internal/data`, `internal/agent`, `internal/protocol`.

**Forbidden dependencies**: `internal/command`, `internal/server` (the HTTP
server itself), `gui/*`. A use-case must never import a caller.

### Four rules

1. **No user I/O.** No `fmt.Println`, `bufio`, bubbletea, lipgloss, or gin
   inside this package. If a use-case needs confirmation or a rendered
   summary, that belongs to the caller, not here.
2. **Request struct in, Result struct + error out.** Never return a
   pre-rendered string. The caller decides how to display the result.
3. **Request/Result are serializable DTOs.** JSON tags, no function fields,
   no live `*config.Config` pointer, no channels. The goal: these types can
   become `swagger.WithRequestModel(...)` inputs directly when an HTTP
   endpoint is added later, so the API schema and the use-case contract are
   the same definition instead of a third hand-written shape.
4. **Typed errors carrying data, not rendered text.** Example: `ErrRuleExists{UUID}`
   lets the CLI print "use `config rule update`", the TUI print "use Edit",
   and a future HTTP handler return 409 with the UUID in the body — from the
   same error value. Every current implementation in `internal/command` uses
   `fmt.Errorf` with hand-written prose baked into the message; that's the
   concrete pattern being replaced.

### No interfaces (yet)

Each domain is one concrete struct — `RuleUseCase`, `ProviderUseCase`,
`AgentUseCase` — holding a `*config.Config` (or `*command.AppManager`-shaped
dependency passed at construction; see below). An interface gets introduced
only once a second implementation (e.g. an HTTP-client-backed use-case for a
remote mode) actually exists. Until then an interface is speculative and adds
a layer of indirection nothing needs.

## Package layout

```
internal/usecase/
  rule.go       RuleUseCase + Request/Result/Error types
  rule_test.go
  provider.go   ProviderUseCase + Request/Result/Error types
  provider_test.go
  agent.go      AgentUseCase + Request/Result/Error types, routing key table
  agent_test.go
```

One file per domain. No shared base type — each use-case is independent;
duplication between them (e.g. similar not-found error shapes) is preferred
over a premature shared abstraction.

## Construction

Use-cases take `*serverconfig.Config` directly (the type callers already have
via `AppManager.AppConfig().GetGlobalConfig()`), not `*command.AppManager` —
`AppManager` lives in `internal/command`, which is on the forbidden-dependency
list precisely because it's a caller, not a foundation. `AppManager` and the
TUI/Web handlers construct a use-case as needed and call into it; the
use-case never reaches back up.

## Migration status

As of Stage 2 (2026-08), `internal/usecase` exists standalone with its own
tests. CLI, TUI, and the Web handler are **not yet migrated** to call it —
that's Stage 3, deliberately deferred (see spec §2.6). Business logic exists
in two places simultaneously during this window: the use-case, and each
caller's original implementation. The one exception is the agent
routing-key table (spec §2.2d) — CLI and TUI call sites were updated to call
the single use-case-owned function, because leaving two hand-copied tables in
place would defeat the point of extracting it.

## Known behavioral differences not yet resolved

Surfaces disagree today; the use-case layer takes a stance (documented per
type below) rather than silently picking one CLI/TUI already had:

- **API style inference** (CLI has it, TUI doesn't)
- **Token masking for confirmation display** (CLI has it; not a use-case
  concern per rule 1 — display formatting is caller I/O)
- **Proxy field** (TUI has it, CLI doesn't)
- **Model list fallback depth**: engine is cache→vmodel→API→template (4
  levels), TUI's own list-building code only did cache→template (2 levels,
  missing virtual providers). `ProviderUseCase.AvailableModels` uses the full
  4-level engine path — this is a deliberate behavior change from the TUI's
  historical shortcut, not a preserved quirk.
