# imbot/core

Platform-neutral vocabulary and base machinery shared by every platform
implementation. Nothing in this package knows about a concrete platform SDK.

## Contents

| File | Responsibility |
|---|---|
| `bot.go` | The `Bot` interface, `SendMessageOptions`, `SendResult` |
| `base.go` | `BaseBot` — status, event handlers, chunking; embedded by all platform bots |
| `message.go` | Inbound `Message` and its `Content` variants |
| `message_builder.go` | Fluent builder over `Message` |
| `types.go` | Platform ids, chat types, parse modes, segments, capabilities |
| `platforms.go` | `PlatformDescriptor` table — the single source of truth for per-platform display names, capabilities, reactions, and behavior defaults |
| `action.go` | `Action` / `ActionSet` — the neutral outbound interactive payload |
| `payload.go` | `Payload` — button identity as ordered segments |
| `segments.go` | Multi-segment (thinking/body) outbound text helpers |
| `restate.go` | `MessageRestater` — "this message is stale, replace it" capability |
| `config.go` | Bot configuration and validation |
| `errors.go` | `BotError` and typed error codes |
| `handler.go` | Generic content-handler registry used by platform adapters |
| `adapter.go` | Adapter base helpers |
| `logger.go` | Injectable per-bot `Logger` interface |
| `media_url.go` | Media URL normalization |

Design rationale for actions, payloads, capabilities, and restate lives in
`.design/imbot-platform-seams.md` at the repo root.
