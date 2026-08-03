# Smart Guide on Claude Code (retiring tingly-agentscope)

> **Superseded — do not build from this.** Kept for the reasoning, not the plan.
> Current design: [`afk.md`](./afk.md).
>
> This plan collapsed `@tb` into a constrained Claude Code subprocess. Its
> premise was that maintaining a second agent runtime (`tingly-agentscope`) was
> the real burden. That burden is gone: agentscope was replaced by `afk/`, an
> in-process runtime we own, and the duplication argument no longer applies.
>
> Three reasons not to revive it:
>
> - **The changedir section is evidence against itself.** The stable-anchor /
>   logical-pwd split exists only to work around the `claude` CLI resolving
>   `--resume` relative to cwd. Inventing a fake anchor directory and then
>   prompting the model to use absolute paths is an impedance mismatch, not a
>   design.
> - **It puts a third-party binary and its private session-file layout on the
>   critical path** of a core product surface. `@tb` could not then run inside
>   the gateway process, the TUI, or the desktop app.
> - **It forfeits the single-protocol depth advantage** described in
>   [`afk.md` §1](./afk.md) — the thing that makes `@tb` something only
>   Tingly-Box can build.
>
> What remains valid: §2's observation that only three tools
> (`change_workdir`, `get_status`, `send_file`) are genuinely tingly-box-specific,
> and §3's conclusion that `@tb` and `@cc` should not share a session.

## Motivation

Smart Guide (`@tb`) today runs on a second, home-grown agent runtime
(`github.com/tingly-dev/tingly-agentscope`): its own ReAct loop, its own
session store, its own tool framework, its own approval path. We already drive
the real `claude` binary for `@cc` through `agentboot`, a battle-tested stack
with subprocess management, a streaming JSON transport, a JSONL session store,
and an `IMPrompter`-based approval flow.

Maintaining two parallel agent runtimes for one product is the actual burden —
`tingly-agentscope` is just the most fragile link in the duplicate. This design
collapses `@tb` into **a constrained Claude Code profile** so `@tb` and `@cc`
differ only by *system prompt + allowed tools + permission mode*, and deletes
`tingly-agentscope` entirely.

The `@tb` v4 system prompt already opens with "You are Claude Code" and its
`read`/`write`/`edit`/`bash` tools mimic Claude's native tools — we are
essentially impersonating Claude Code already. This makes it the real thing.

## Scope of the dependency (clean removal boundary)

`tingly-agentscope` is imported only under:
- `remote/control/smart_guide/` (10 files)
- `remote/control/bot/{agent_smart_guide.go,bot_agent.go}` (2 files)

Nothing else in the tree depends on it.

---

## Core design decisions (settled)

### 1. `@tb` = constrained Claude Code, not a separate runtime

`SmartGuideExecutor.Execute` is rewritten to mirror `ClaudeCodeExecutor`: it
calls `AgentService.Run(...)` with `ExecutionOptions` that differ from `@cc`
only in:

- `AppendSystemPrompt` — the `@tb` guide role (reuse `prompts/`).
- `AllowedTools` — `Read, Write, Edit, Bash` (native) + the three MCP tools
  below. No `Task`/`WebFetch`/etc. — keeps the "navigate + simple edits"
  contract.
- `PermissionMode` — stricter default for `@tb` (e.g. require approval for
  Bash writes), routed through the same `IMPrompter` + `PermissionPromptTool:
  "stdio"` path as `@cc`.
- `Env` — `GetClaudeCodeEnv` (gateway), same as `@cc`.

`read`/`write`/`edit`/`bash` from `tingly-agentscope/extension/tools` are
**dropped** in favor of Claude's native tools.

### 2. The three tingly-box-native tools become one MCP server

Only three tools are genuinely tingly-box-specific. They move into a small
**MCP-over-HTTP server** built on `github.com/mark3labs/mcp-go` (already a
dependency) and injected via `ExecutionOptions.MCPServers` (handled in
`cli_builder.go:135-150` → `--mcp-config`).

| Tool | Behavior (reuses existing callbacks) |
|---|---|
| `change_workdir(path)` | Update the chat's **logical pwd** in `ChatStore`; return canonical absolute path. (reuse `UpdateProjectFunc` logic, `agent_smart_guide.go:133-143`) |
| `get_status()` | Read `ChatStore` (project path, logical pwd, whitelist). (reuse `GetStatusFunc`, `agent_smart_guide.go:114-129`) |
| `send_file(path, caption)` | Call back into the IM bot. (reuse `e.deps.SendFile`) |

**Per-chat binding:** each execution injects the MCP server URL plus a
**per-execution token** (header). The MCP handler resolves `token → chatID`,
then reads/writes the correct `ChatStore` entry. The tools are stateless RPCs;
all state lives in tingly-box.

### 3. No session sharing between `@tb` and `@cc`

Each keeps an independent Claude session (independent transcript, toolset,
permission contract). They are bridged by:
- shared `ChatStore` (logical pwd / project history), and
- an explicit **handoff payload** on `@tb → @cc` (reuse `handoff_to_cc_prompt`:
  current dir, user goal, key files).

Rationale: a shared transcript would mix two roles/toolsets (model sees tool
calls for tools it no longer has), muddies per-session approval/audit, and is
hard to un-share later. cwd continuity does **not** require session sharing —
it lives in `ChatStore`, read by both.

### 4. changedir: decouple "where the session lives" from "where we work"

This is the crux. Claude couples both onto one `ProjectPath`:
- `driver.go:158` → `WorkDir = opts.ProjectPath` (the process cwd)
- `driver.go:223` → `--resume <id>`, which the `claude` CLI resolves **relative
  to cwd** (`~/.claude/projects/<encoded-cwd>/`, see `session/path.go`).

So if cwd floats, `--resume` looks in the wrong project dir and the conversation
breaks. But `@tb`'s defining design is "session preserved, pwd floats freely per
turn." We reconcile by **not letting the two axes be equal**:

| Axis | Bound to | Behavior |
|---|---|---|
| **Session anchor** (= `ExecutionOptions.ProjectPath` / cmd.Dir / `--resume` lookup) | a **stable per-chat dir**, e.g. `~/.tingly/chats/<chatID>/`, never changes | `--resume` always resolves → "session preserved" |
| **Logical pwd** (where the user is working) | `ChatStore.BashCwd`, floats freely | `change_workdir` only moves this → "dir changes, independent pwd per turn" |

Per turn, `SmartGuideExecutor`:
1. launches claude with `ProjectPath` = the **stable anchor** (not the logical
   pwd), `SessionID` = the chat's `@tb` session, `Resume = true`;
2. injects the logical pwd into the turn (prompt prefix / `get_status`):
   "your current working directory is `<X>`";
3. passes `--add-dir <logical pwd>` so Claude may access files there;
4. relies on the system prompt to make the model operate via **absolute paths**
   (and/or `cd <logical pwd> && ...` inside a single Bash call).

`change_workdir` updates only `ChatStore.BashCwd` and **never touches the
anchor**, so it cannot break `--resume`.

Mental model = a shell: the session persists, `pwd` wanders.

**Wrinkle to validate in the prototype:** Claude's Bash relative commands run in
the anchor, not the logical pwd. Mitigations: system-prompt the model to use
absolute paths or `cd` within a Bash call; `--add-dir` for file-tool access.
`@tb` is a navigation assistant, so this constraint is acceptable.

---

## Implementation phases

### Phase 0 — Spike: changedir continuity (de-risk the crux)
Prove the anchor/logical-pwd split end to end before deleting anything.
- Add a minimal `SmartGuideExecutor` path that runs `AgentService.Run` with:
  guide `AppendSystemPrompt`, `AllowedTools = [Read,Write,Edit,Bash]`, stable
  anchor as `ProjectPath`, `--add-dir <logical pwd>`, logical pwd injected.
- MCP tools stubbed (or only `send_file` real).
- Manually verify across turns: change "dir", confirm session resumes, confirm
  files at the new logical pwd are readable, confirm pwd reported correctly.
- **Decision gate:** absolute-path-only vs `cd`-in-bash vs `--add-dir` — pick
  what feels right, lock it into the system prompt.

### Phase 1 — tingly-box MCP server
- New package (e.g. `remote/control/smart_guide/mcpserver/`) using
  `mark3labs/mcp-go`, mounted on the existing HTTP server.
- Three tools: `change_workdir`, `get_status`, `send_file` — handlers call the
  existing `ChatStore` / `SendFile` callbacks.
- Per-execution token → `chatID` resolution.
- Inject via `ExecutionOptions.MCPServers` + `AllowedTools` entries
  (`mcp__tinglybox__*`).

### Phase 2 — Rewrite `SmartGuideExecutor`
- Replace the `tingly-agentscope` agent construction/execution with
  `AgentService.Run`, modeled on `ClaudeCodeExecutor`.
- Build the guide `AppendSystemPrompt` from `prompts/` (strip the redundant
  "You are Claude Code" preamble; keep role/workflow/handoff/scope).
- Wire MCP server + token; wire stable anchor + logical pwd injection.

### Phase 3 — Session plumbing
- `@tb` runs through `agentboot/claude` sessions, in a **separate namespace**
  from `@cc` (e.g. `tb:`-prefixed session id) so the two never collide.
- Update `agent_router.go:53-57` (drop the chatID-as-sessionID special case;
  `@tb` now resolves a real claude session via `resolveSession`, anchored to the
  stable per-chat dir).
- Delete `smart_guide/session_store.go` and `TBSessionStore`.

### Phase 4 — Approval unification
- Drop `smart_guide`'s `Approver` / `ToolContext.RequestApproval`.
- `@tb` tool approvals go through the same `IMPrompter` + `stdio` path as `@cc`.

### Phase 5 — Delete tingly-agentscope
- Remove `agent.go`, `handler.go`, `session_store.go`, `tools*.go` and their
  tests from `smart_guide/` (keep `prompts/`).
- Remove agentscope imports from `agent_smart_guide.go` / `bot_agent.go`.
- `go.mod`: drop `github.com/tingly-dev/tingly-agentscope`; `go mod tidy`.

### Phase 6 — Tests & docs
- Port the meaningful `smart_guide` tests to the new path (MCP tool behavior,
  changedir continuity, approval routing).
- Update any docs referencing the agentscope-based `@tb`.

---

## Open questions / decisions still needed
1. **Stable anchor dir**: `~/.tingly/chats/<chatID>/` vs the chat's initial
   project root vs a shared `@tb` home. (Phase 0 input.)
2. **Within-turn navigation**: absolute-path-only vs `cd`-in-bash vs `--add-dir`
   — resolved by the Phase 0 spike.
3. **Handoff session linkage**: independent sessions confirmed; finalize the
   handoff payload contents.

## Result
One runtime (`agentboot/claude`), one session store, one approval path, one
streaming path. `@tb` and `@cc` become two profiles over the same engine, and
`tingly-agentscope` is gone.
