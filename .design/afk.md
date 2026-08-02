# AFK — Agent Framework Kit

> The in-process agent runtime behind Smart Guide (`@tb`). Lives in `afk/`, its
> own Go module inside the repo.
>
> Read this before changing `afk/` or `internal/remote_control/smart_guide/`. It
> records the decisions that are not visible from the code, and the ones that
> look wrong until you know what they are protecting against.
>
> Supersedes the north star in [`smart-guide-on-claude-code.md`](./smart-guide-on-claude-code.md)
> (see *Retiring the Claude Code plan* below). Continues
> [`smart-guide-react-anthropic-sdk.md`](./smart-guide-react-anthropic-sdk.md),
> which describes how AFK came to exist.

---

## 1. One protocol, on purpose

AFK speaks the Anthropic Messages API and nothing else. Messages are the SDK's
native `anthropic.BetaMessageParam`; there is no neutral message type, no
provider abstraction, no conversion layer.

This is not a limitation we tolerate — it is the point.

Tingly-Box **is** the provider abstraction, at the wire level, in
`internal/protocol/`, load-tested by `harness-duo` and `harness-matrix`. Any
model the user configures is reachable through the gateway as Anthropic-shaped
traffic. Building a second provider abstraction inside the agent would duplicate
the gateway's core capability and create a second source of truth for it.

The payoff is depth. A framework that must serve several providers takes the
intersection of what they all support: tool streaming, cache control, thinking
blocks, and stop-reason semantics all degrade to whatever the weakest one does.
Because AFK only ever talks to one shape, it can use the whole surface — and
does: prompt-cache breakpoints, thinking blocks preserved with their signatures,
native `tool_use` / `tool_result` blocks, per-turn usage accounting.

**Invariant: no provider-compat layer enters `afk/`.** If a model cannot be
reached as Anthropic-shaped traffic, that is the gateway's problem to solve, not
the agent's.

---

## 2. Architecture

```
                      ┌──────────────────────────────────────┐
  smart_guide  ──────▶│ Harness                              │
  (tools, prompts,    │  run loop, checkpoints, steering,    │
   approval, IM glue) │  compaction, replay invariants       │
                      └───────────────┬──────────────────────┘
                                      │
                   ┌──────────────────┴───────────────────┐
                   ▼                                      ▼
        ┌────────────────────┐                ┌──────────────────────┐
        │ Engine.Step        │                │ session.Session      │
        │ one model call +   │                │ append-only JSONL    │
        │ the tool batch it  │                │ message / compaction │
        │ requested          │                │ entries              │
        └────────────────────┘                └──────────────────────┘
```

| Layer | File | Owns |
|---|---|---|
| `Engine` | `afk/engine.go` | The model client, tools, thinking/effort, cache breakpoints, one step |
| `Harness` | `afk/harness.go` | The run loop, checkpoints, steering, compaction, invariants |
| `Session` | `afk/session/session.go` | The durable log and its projection |
| Compaction | `afk/compact.go` | Boundary selection and summarization |
| Truncation | `afk/truncate.go` | Tool-output caps |
| Skills | `afk/skill/` | Discovery, parsing, the `activate_skill` tool |

`Engine.Run` still exists with its original signature; it is a thin wrapper over
a `Harness` with no log. Nothing that needs to act *between* steps —
persistence, steering, compaction — can live inside a model call, which is why
the loop moved up a layer.

**Terminology.** A *step* is one model call plus the tool batch it requested. A
*run* is one user turn: a sequence of steps until the model answers without
calling tools. A *checkpoint* is the point between steps where the conversation
is consistent and work can be persisted, steering consumed, and compaction
considered.

---

## 3. Decisions that are easy to get wrong

Each of these looks like it could be simpler. Each is the shape it is because
the simpler version breaks something.

### 3.1 The transcript may not end on a user turn

An interrupted run ends on either the prompt itself or the last step's tool
results — both user turns. Appending the next prompt on top puts two user
messages in a row, which the API rejects. Because runs are now persisted
mid-flight, that state would be written to disk and the conversation would be
wedged permanently, with no recovery but `/clear`.

The harness closes an interrupted run with a short assistant note. The user
prompt is likewise held back and flushed together with the first step, so a
first-step failure cannot leave a bare user message on disk.

**If you add a path that ends a run, it goes through `Harness.close`.**

### 3.2 Cache breakpoints roll, and are never written in place

`cache_control` is a **per-content-block field in the request body**. There is no
header that turns caching on: prompt caching has been GA since the
`prompt-caching-2024-07-31` beta was retired, and the 1h TTL is a `ttl` field on
the `cache_control` object, not a beta flag either. The only request-level form
is `BetaMessageNewParams.CacheControl`, which auto-places a *single* breakpoint on
the last cacheable block — strictly less than what we place by hand, so we don't
use it.

Two breakpoints, out of a per-request budget of four:

1. **The static prefix** — on the system prompt, which covers the tools too,
   since the request renders `tools → system → messages` and a breakpoint caches
   everything up to its own block. Only when there is no system prompt do we fall
   back to marking the last tool.
2. **The conversation tail** — on the final content block, moving forward every
   request.

The rolling one is not an optimization. A breakpoint searches backwards **at most
20 content blocks** for an existing cache entry, and one tool-heavy turn can
append more blocks than that — a pinned breakpoint would go cold exactly on the
runs that cost the most. Dropping it instead of rolling it is worse still: the
message history would be re-processed at full price on every step, and in an
agentic run that history is the *large* half of the prompt (it grows to the
compaction trigger, §3.7), while the static prefix is the small one.

The breakpoint is applied to a **copy** of the tail. Message content blocks are
pointers into the caller's history, which Smart Guide persists verbatim; writing
through would leak breakpoints into the stored session and accumulate them until
a request exceeded the per-request limit. Two tests fail if the copy is removed.

**Verification is `Usage.CacheReadInputTokens`.** Zero across repeated requests
with a stable prefix means something is invalidating it. The prefix is
byte-stable today by construction: the system prompt is an embedded static file
(`default_system_prompt.v5`, no timestamp interpolation), `BuildTools` returns a
fixed-order slice rather than a map walk, and skill names are sorted (§3.5).
Anything that makes one of those vary per request silently costs the whole cache.

### 3.3 Reasoning is not an answer

Thinking text goes to `StreamSink.OnThinking`, never `OnText`, and never becomes
a run's final output. It used to be substituted in when a turn produced no text
— the common shape for that is *think, then call a tool*, which has not answered
anything — so `@tb` published the model's private deliberation to the chat as
though it were the reply.

Thinking blocks stay in history **with their signatures**, which the API
validates on replay. A block that does not survive the session store's JSON
round trip is a rejected request on the user's next message.

Whether the model thinks at all is one config field, `Config.Thinking`, with
four values that each map to exactly one request shape:

| `ThinkingMode` | sends |
|---|---|
| `ThinkingModelDefault` (zero value) | nothing — the model's own default applies |
| `ThinkingVisible` | `{type: adaptive, display: summarized}` |
| `ThinkingHidden` | `{type: adaptive, display: omitted}` |
| `ThinkingOff` | `{type: disabled}` |

One field rather than an on/off flag plus a visibility flag, because those are
not independent here: reasoning you cannot see and no reasoning at all are
different requests, and *unset* is a third thing again — on current models
omitting the parameter already means adaptive thinking, on slightly older ones
it means none.

**`ThinkingVisible` is the one that makes `OnThinking` useful.** `display`
defaults to `omitted` on current models, so thinking blocks otherwise arrive
with empty text and the 💭 rendering never has anything to show. Asking for
reasoning and asking to see it are the same decision in practice, which is why
they are one value and not two.

The default stays *unset*: `@tb` routes to whatever model the user configured,
and a mode a model rejects is a hard failure, while its own default is by
definition supported. An unrecognized value falls back to sending nothing —
the mode arrives as a plain string from `SmartGuideConfig.Thinking`, and a typo
must not become a 400.

### 3.4 Effort is a separate knob from thinking

`Config.Effort` maps to `output_config.effort` — `low` / `medium` / `high` /
`xhigh` / `max`, with the zero value sending nothing. It is the API's primary
intelligence/latency/cost control: it governs how much the model spends on the
whole response, not only on reasoning.

It stays its own field rather than more values on `ThinkingMode`, because the two
are genuinely orthogonal: thinking is *whether the model reasons and whether we
see it*, effort is *how much of everything it spends*. Folding them together
would produce a twenty-value picker whose cells mostly mean the same request.

They interact only at the edges — on some models, disabling thinking is rejected
above `high` effort. AFK cannot check that: it does not know which model the
gateway routed to. As with thinking, an unrecognized value sends nothing rather
than becoming a 400, because the value arrives as a plain string from
`SmartGuideConfig.Effort`.

### 3.5 Skills are discovered once per agent

The skill catalog is rendered into the `activate_skill` tool description, and
tools sit at the very front of the prompt. A catalog that changed mid-conversation
would invalidate the prompt cache for the entire history on the turn it changed.
Names are sorted so the rendered definition is byte-stable across restarts.

`@tb`'s working directory floats, so this does mean walking into a directory
with skills does not pick them up until the session restarts. That is the
accepted cost of a stable prefix; a fresh agent is built per session, so
`/clear` or a restart picks up changes.

### 3.6 Tool output is capped, and the direction is per tool

Two independent limits, whichever binds first: 2000 lines
(`DefaultMaxOutputLines`) or 50KB (`DefaultMaxOutputBytes`). The byte cap is
what actually protects the context — one 200KB line is as damaging as ten
thousand short ones — while the line cap keeps ordinary output readable.

`bash` keeps the **tail** (a failing command reports its error at the bottom);
`read` keeps the **head** (matching what reading from an offset means). Every
truncated result carries a notice saying what was dropped and how to get the
rest. Silent truncation is worse than none: the model reasons from a partial
result as if it were complete.

**Ordering hazard in `bash`:** truncation runs *after* the trailing `pwd` line is
stripped. That line is how the tool tracks the working directory; tail-truncating
first would cut it off and strand the session in the previous directory. There is
a test for exactly this.

### 3.7 Compaction is the platform's, not ours

Server-side compaction from the Beta Messages API, on by default:
`context_management` carrying a `compact_20260112` edit in the body, plus the
`compact-2026-01-12` flag as a header. Trigger pinned to
`DefaultServerCompactTrigger` = 120k rather than inheriting the API's own
default, so the number `@tb` runs at is visible in our code and cannot move
underneath us. `Instructions` steers the summary toward the state a tool-using
agent needs; `PauseAfterCompaction` stays false so compaction is invisible to
the run rather than a step to resume.

**We do not rewrite history.** The API returns a compaction block in the
assistant message; we keep sending the full conversation, and that block is what
tells the API which part of it to replace next time. So there is nothing to
summarize ourselves, no boundary to choose, and no compaction record in the log —
the block persists as part of the message, like any other content block.

That makes one test load-bearing: **the compaction block has to survive both
history and the session log's JSON round trip**
(`TestHarness_CompactionBlockSurvivesHistoryAndPersistence`). A block dropped
anywhere means the next request re-sends the conversation the compaction was
supposed to shrink.

An earlier revision hand-rolled this: an extra model call per compaction, a
summarization prompt of our own, and a boundary rule that had to avoid cutting a
`tool_result` away from its `tool_use`. It was deleted. Using the beta API is the
one-protocol thesis (§1) applied — the reason to speak one protocol is to use its
whole surface, not to reimplement it.

### 3.8 Steering lands where the transcript allows

A message sent while a turn is running is delivered at the next checkpoint.
Where it lands depends on what the step ended with: after a tool step it joins
the `tool_result` user message; after a text step it becomes a fresh user
message **and the run continues**, because the model has not seen what the user
just said.

Steering is merged *before* the checkpoint writes, so the log and the in-memory
conversation never disagree about what the model was shown. Anything still
queued when a run ends is dropped rather than carried into the next turn, where
it would answer a question nobody is asking any more.

Only `@tb` registers as steerable. `@cc` drives a subprocess and still falls
through to the busy path.

### 3.9 The log is append-only

One JSONL entry per line, `O_APPEND`, never rewritten. An unreadable line costs
one message rather than the conversation — the previous whole-file store treated
any parse failure as "no history at all". The scanner is sized for turns carrying
tool output; `bufio`'s 64KB default would stop mid-file and take the rest of the
history with it.

Entries are typed even though there is currently only `message`. The envelope is
the file format: adding a kind of record later should not mean rewriting every
log that already exists. Compaction does not need one (§3.7).

A persistence failure is logged, never returned. The run's work is real whether
or not it was recorded, and failing the turn over a disk error would discard
tool side effects it cannot undo.

---

## 4. What we deliberately did not build

The reference for this layer is [pi](https://github.com/earendil-works/pi)
(`packages/agent`), whose split of *step primitives / harness / session /
storage* is the one adopted here. Its `packages/ai` — a large multi-provider
layer — is the part deliberately **not** taken, for the reason in §1.

Also not taken, and not currently wanted:

| Not built | Why not |
|---|---|
| Durable resume across process restarts | Requires orchestration entries and recovery logic. A dropped turn is recoverable by asking again; the checkpoint already keeps the work. |
| Conversation branching / tree navigation | No product surface asks for it. |
| Refs (parallel operations on one session) | A chat is one conversation at a time. |
| Hooks / extension points | Approval is wired directly today. Worth revisiting when there is a second consumer — see §6. |
| An in-process compaction fallback | The beta API does it (§3.7). A second mechanism would need a threshold ordered against the server's trigger, and would fire on exactly the conversations where its worse summaries hurt most. |

The guiding rule: the loop and the checkpoint are what everything else needs.
The rest is weight this product has no use for yet.

---

## 5. Known limits and open questions

**Beta features are the default.** AFK talks to the Beta Messages API and uses
what it offers rather than reimplementing it. Compaction is the first case; the
`Config.DisableServerCompaction` escape hatch exists for an upstream that cannot
take it, and turning it off means nothing bounds the prompt.

**The compaction trigger is a fixed token count**, not a fraction of the model's
context window, because `@tb` routes by bot-UUID rule and the window is not known
at runtime. 120k suits the models compaction is available on (all 1M-window) and
would be too high for a small one.

**We mark one message, Claude Code marks two.** Read off its shipped bundle
(`claude-code/cli.js`), its rule is `J > A.length - 3` — a breakpoint on the last
content block of each of the **last two** messages, user and assistant alike,
skipping a `thinking` / `redacted_thinking` tail. The second one is the answer to
the same 20-block problem §3.2 describes, from the other side: an older
breakpoint sitting exactly on the previous request's write point is a precise hit
that does not depend on the lookback window at all, so a turn that appended more
than 20 blocks still reads. Its system handling matches ours — every dynamic
system fragment is joined into one block and marked once, so the effective static
prefix is a single breakpoint. Budget-wise it runs 2 + 2 = 4, exactly the cap;
we are at 2 and a third fits. *Not adopted yet; a deliberate open item.*

**1h TTL and cache scope stay off, and that is Claude Code's default too.** Its
`cache_control` factory emits a bare `{type: "ephemeral"}` — 5 minutes — and adds
`ttl: "1h"` only for query sources on a remote-config allowlist, or on Bedrock
behind an env var; `scope: "global"` is likewise gated on a flag plus
`CLAUDE_CODE_FORCE_GLOBAL_CACHE`. A 1h write costs 2× against 5m's 1.25× and
needs three reads to break even rather than two, and `@tb` has neither a query
-source dimension to allowlist by nor knowledge of the model it was routed to.
Off is the defensible default; the escape hatches upstream exist for callers who
can make that judgement per route, and we cannot.

**The minimum cacheable prefix is model-dependent and not monotonic** — 512
tokens on Opus 5, 1024 on Opus 4.8 and Sonnet 5, 2048 on Opus 4.7, 4096 on
Opus 4.6 and Haiku 4.5. Below it nothing caches and **nothing errors**
(`cache_creation_input_tokens` is simply 0). Same shape of problem as the
compaction trigger: the gateway routes by bot-UUID rule, so AFK cannot know which
threshold applies. A short system prompt may quietly cache on one model and not
another.

**Thinking and effort are configurable but unset by default** (§3.3, §3.4) —
"unset" meaning the parameter is not sent, which on current models still means the
model thinks. The default is unset rather than `ThinkingVisible` or a named effort
for the same reason the trigger is a fixed number: the model is not known at
runtime, and its own default is the only setting guaranteed to be accepted.

**Thinking and effort can conflict on some models** (§3.4): disabling thinking is
rejected above `high` effort. AFK sends both as configured and does not
cross-validate them, because the pair is only invalid for certain models and AFK
does not know which model the gateway routed to. Both default to unset, so the
conflict requires setting both explicitly.

**`Temperature` is sent unconditionally** (`smart_guide/agent.go`, from
`SmartGuideConfig`). Current Anthropic models — Opus 5, 4.8, 4.7, Fable 5,
Sonnet 5 — **reject `temperature` with a 400**. Whether the gateway's transform
layer strips it has not been verified. If it does not, `@tb` cannot work on
those models at all. *Unresolved; flagged deliberately.*

**`afk/skill` tests were failing for some time before anyone noticed** — they
pointed at `../../../.agent/skills`, both the wrong depth and the wrong
directory name, stale from when the package lived under `internal/`. Fixed, but
it means the package went unverified across a move. Worth remembering when
moving packages between modules.

---

## 6. If the next step is hooks

Not built, and not needed yet. What would justify it: a second consumer of AFK
inside the repo, or a guardrails requirement that cannot be expressed as a tool.

The shape that would fit is pi's split — *events are passive observations, hooks
are awaited interception points*.

The concrete thing a pre-tool hook would clean up is approval, which today runs
through **two unrelated mechanisms**:

- `ToolExecutor.onApproval` (`SetApprovalCallback`, wired in
  `createApprovalCallback`) gates non-allowlisted `bash` commands.
- `ToolContext.RequestApproval` gates `send_file`, on a different signature,
  set by the executor.

A tool added tomorrow that needs gating would have to pick one, or invent a
third. One interception point in the harness, applied to every tool before it
runs, would collapse both — and give guardrails (`internal/guardrails`, which
does not touch `@tb` today) somewhere to attach.

That is the argument *for*. The argument for waiting is that both mechanisms
work, and building the extension point before there is a second consumer of AFK
means designing it against a single caller.

---

## 7. Checked against `.design/ux-principles.md`

- **#6 smart defaults over toggles** — skills register when a skill directory
  exists, with no enable flag; steering is always on; compaction is on with a
  pinned trigger and no per-chat setting. Both compaction knobs
  (`DisableServerCompaction`, `ServerCompactTrigger`) are escape hatches, not
  settings anyone is expected to tune. Prompt caching is the same call in the
  other direction: breakpoints are placed unconditionally with no flag, while 1h
  TTL and cache scope stay off because turning them on needs a per-route
  judgement `@tb` has no basis to make (§5).
- **#7 diagnostics must traverse the real path** — cache health is read from
  `Usage.CacheReadInputTokens` on the actual response, not inferred from the fact
  that we set the breakpoints. A silent invalidator (§3.2) produces a correct
  request that simply never reads, and only the real number shows it.
- **#4 separate orthogonal axes / #2 no mode pickers** — thinking and effort are
  the two genuine multi-value settings, and the principle cuts both ways here.
  Thinking stays a *single* field because its values are not orthogonal (§3.3):
  splitting it into on/off plus visible/hidden would invent a fourth combination
  the API has no shape for, and would hide that *unset* differs from both.
  Effort stays a *separate* field for the mirror-image reason (§3.4): it is a
  different axis — how much is spent, not whether we reason — so merging it into
  thinking would produce a mode picker of mostly-identical cells.
- **#10 done ≠ locked** — `/clear` archives rather than deletes, and the
  append-only log never rewrites what it already wrote.
- **#12 side effects scoped to the current surface** — a steering message
  affects the running turn only; anything left queued when it ends is dropped.

The steering work is the direct application of the UX-first rule: rejecting a
user's follow-up as a concurrency error was convenient for the implementation
and wrong for the product.
