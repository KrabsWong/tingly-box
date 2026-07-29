package afk

import (
	"context"
	"math"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
)

// Log is where a Harness persists conversation as it is produced. A nil Log
// means the caller does not want durability, not that persistence failed.
//
// *session.Session satisfies this; so does any adapter a host wants to put in
// front of it.
type Log interface {
	Append(msgs ...anthropic.BetaMessageParam) error
	// AppendCompaction records that the first replaced messages of the
	// conversation are now represented by summary.
	AppendCompaction(summary string, replaced int) error
}

// RunResult is the outcome of one user turn.
type RunResult struct {
	// Messages is the full conversation: the history it was given, plus
	// everything this run produced.
	Messages []anthropic.BetaMessageParam
	// FinalText is the assistant text the run ends on, empty when the run
	// produced none (a tool-only run, or one cut short).
	FinalText string
	Usage     Usage
	Steps     int
	// Steered counts mid-run messages the user sent that this run picked up.
	Steered int
	// Compactions counts how many times this run had to summarize its own
	// history to keep the prompt inside the budget.
	Compactions int
	// Interrupted reports that the run stopped without the model producing a
	// tool-free answer — cancelled, failed, or out of step budget.
	Interrupted bool
}

// Harness drives a run: it sequences steps, checkpoints between them, and owns
// the invariants the conversation has to satisfy to be replayable.
//
// It deliberately does not do durable resume, branching, or crash recovery. The
// loop and the checkpoint are what everything else needs; the rest is weight
// this product has no use for yet.
type Harness struct {
	engine *Engine
	log    Log

	mu      sync.Mutex
	running bool
	queued  []string
}

// NewHarness builds a Harness over an engine. log may be nil.
func NewHarness(e *Engine, log Log) *Harness {
	return &Harness{engine: e, log: log}
}

// Steer hands the running turn an additional message from the user, to be
// delivered at the next checkpoint. It reports whether a run was there to take
// it; false means the caller should start a normal run instead.
//
// This is the difference between a chat and a batch job. People send follow-ups
// while the agent is still working — "wait, not that directory", "also check the
// logs" — and the alternative to accepting them is rejecting them as
// concurrent, which asks the user to sit still and watch.
//
// It is safe to call from another goroutine while Run is executing; that is the
// only way it is ever called.
func (h *Harness) Steer(text string) bool {
	if text == "" {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.running {
		return false
	}
	h.queued = append(h.queued, text)
	return true
}

// drainSteering removes and returns any queued messages.
func (h *Harness) drainSteering() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	q := h.queued
	h.queued = nil
	return q
}

// setRunning marks the run active so Steer knows there is something to steer.
// Anything still queued when a run ends is dropped: it belonged to a turn that
// is over, and replaying it into the next one would answer a stale question.
func (h *Harness) setRunning(v bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.running = v
	if !v {
		h.queued = nil
	}
}

// injectSteering merges steering messages into the tail of a step's output.
//
// Where it lands depends on what the step ended with, because the transcript
// has to keep alternating. After a step that called tools the tail is a user
// turn carrying tool results, so the text joins that message rather than
// starting a second user turn beside it. After a step that only produced text
// the tail is an assistant turn, and a fresh user message is exactly right.
func injectSteering(stepMsgs []anthropic.BetaMessageParam, steering []string) []anthropic.BetaMessageParam {
	blocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(steering))
	for _, s := range steering {
		blocks = append(blocks, anthropic.NewBetaTextBlock(s))
	}

	out := append([]anthropic.BetaMessageParam(nil), stepMsgs...)
	if n := len(out); n > 0 && out[n-1].Role == anthropic.BetaMessageParamRoleUser {
		last := out[n-1]
		// Copy the content slice: it is shared with the caller's view of the
		// conversation, and appending in place could write through to it.
		merged := make([]anthropic.BetaContentBlockParamUnion, 0, len(last.Content)+len(blocks))
		merged = append(merged, last.Content...)
		merged = append(merged, blocks...)
		last.Content = merged
		out[n-1] = last
		return out
	}
	return append(out, anthropic.NewBetaUserMessage(blocks...))
}

// interruptedNote is appended when a run ends without the model replying.
//
// It exists to keep the conversation replayable, not to inform the user: the
// API wants alternating roles, and a run cut short leaves the transcript ending
// on a user turn — either the prompt itself, or the tool results from the last
// step. Appending a user message on the next turn would then put two in a row.
// It doubles as context the model can see next time.
const interruptedNote = "[This turn was interrupted before I produced a reply.]"

// Run executes one user turn to completion, or until it is cancelled or runs
// out of steps.
//
// Messages are checkpointed to the log as each step completes rather than only
// at the end, so an interrupted run keeps the work it already did — including
// the tool calls whose side effects already happened on the user's machine.
func (h *Harness) Run(
	ctx context.Context,
	history []anthropic.BetaMessageParam,
	userText string,
	sink StreamSink,
) (RunResult, error) {
	e := h.engine

	h.setRunning(true)
	defer h.setRunning(false)

	messages := append([]anthropic.BetaMessageParam(nil), history...)
	userMsg := anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(userText))
	messages = append(messages, userMsg)

	// The user message is held back and flushed with the first step's output.
	// Persisting it on its own would leave the transcript ending on a user turn
	// if that first model call then failed, which is the shape the note above
	// exists to prevent.
	pending := []anthropic.BetaMessageParam{userMsg}

	res := RunResult{}

	logrus.WithFields(logrus.Fields{
		"model":         e.model,
		"history_msgs":  len(history),
		"prompt_len":    len(userText),
		"tools":         len(e.tools),
		"maxIterations": e.maxIterations,
		"stream_text":   e.streamText,
		"persisted":     h.log != nil,
	}).Debug("afk harness: run start")

	for i := 0; i < e.maxIterations; i++ {
		if err := ctx.Err(); err != nil {
			logrus.WithError(err).WithField("step", i).Debug("afk harness: context cancelled")
			res.Messages, res.Interrupted = h.close(messages, pending), true
			return res, err
		}

		step, err := e.Step(ctx, messages, sink)
		if err != nil {
			res.Messages, res.Interrupted = h.close(messages, pending), true
			return res, err
		}

		// Checkpoint. Steering is merged into the step's output before anything
		// is recorded, so the log and the in-memory conversation never disagree
		// about what the model was shown.
		stepMsgs := step.Messages
		steering := h.drainSteering()
		if len(steering) > 0 {
			stepMsgs = injectSteering(stepMsgs, steering)
			res.Steered += len(steering)
		}

		messages = append(messages, stepMsgs...)
		pending = append(pending, stepMsgs...)
		res.Steps++
		res.Usage.Add(step.Usage)
		if step.Text != "" {
			res.FinalText = step.Text
		}

		logrus.WithFields(logrus.Fields{
			"step":       i,
			"turn_text":  len(step.Text),
			"tool_calls": step.ToolCalls,
			"steering":   len(steering),
		}).Debug("afk harness: step complete")

		// The step's messages are well-formed now, so they can be made durable
		// before the next model call is risked.
		h.checkpoint(&pending)

		// Compaction, if the prompt has grown past the budget. This is the only
		// place it can happen: between steps, where the conversation is
		// consistent and nothing is mid-flight.
		if promptTokens(step.Usage) > h.compactAt() {
			messages = h.compact(ctx, messages, &res)
		}

		// Steering keeps the run going even when the model was ready to stop:
		// the user has said something the model has not seen, and ending here
		// would strand it until they typed again.
		if len(steering) > 0 {
			continue
		}

		if !step.NeedsAnotherStep {
			res.Messages = messages
			logrus.WithFields(res.Usage.LogFields()).WithFields(logrus.Fields{
				"steps":     res.Steps,
				"final_len": len(res.FinalText),
			}).Info("afk harness: run complete (final answer)")
			return res, nil
		}
	}

	// Budget exhausted while the model was still calling tools. Loud, because a
	// run that spent every step and produced no reply is a user-visible
	// non-answer and needs to be greppable.
	res.Messages, res.Interrupted = h.close(messages, pending), true
	logrus.WithFields(res.Usage.LogFields()).WithFields(logrus.Fields{
		"model":         e.model,
		"maxIterations": e.maxIterations,
		"final_len":     len(res.FinalText),
		"had_text":      res.FinalText != "",
	}).Warn("afk harness: hit max iterations without a tool-free final answer")
	return res, nil
}

// promptTokens is how large the request actually was, as the model reported it.
// Cached tokens still occupy the context window, so they count here even though
// they cost almost nothing.
func promptTokens(u anthropic.BetaUsage) int64 {
	return u.InputTokens + u.CacheReadInputTokens + u.CacheCreationInputTokens
}

// compactAt returns the configured prompt-size trigger. A negative setting
// disables compaction by putting the trigger out of reach.
func (h *Harness) compactAt() int64 {
	switch {
	case h.engine.compactAtTokens < 0:
		return math.MaxInt64
	case h.engine.compactAtTokens == 0:
		return DefaultCompactAtTokens
	default:
		return h.engine.compactAtTokens
	}
}

// compact replaces the older part of the conversation with a summary, in place,
// so the next step's request fits.
//
// Every failure path returns the conversation untouched. A conversation that is
// too long still gets one more chance at an answer; one that has been mangled
// by a half-applied compaction is broken for good.
func (h *Harness) compact(
	ctx context.Context,
	messages []anthropic.BetaMessageParam,
	res *RunResult,
) []anthropic.BetaMessageParam {
	boundary := compactionBoundary(messages, compactKeepMessages)
	if boundary == 0 {
		// No clean turn boundary in the tail — the whole recent stretch is one
		// unbroken tool loop. Cutting inside it would orphan a tool_result from
		// its tool_use, which is worse than an oversized prompt.
		logrus.WithField("messages", len(messages)).
			Warn("afk harness: prompt over budget but no safe compaction boundary")
		return messages
	}

	summary, err := h.engine.Summarize(ctx, messages[:boundary])
	if err != nil {
		logrus.WithError(err).Warn("afk harness: compaction failed, continuing uncompacted")
		return messages
	}

	if h.log != nil {
		if err := h.log.AppendCompaction(summary, boundary); err != nil {
			// The log and the live conversation would now disagree about what
			// the model has seen, and the log is what the next turn is rebuilt
			// from. Keep them in step by not compacting.
			logrus.WithError(err).Error("afk harness: could not record compaction, continuing uncompacted")
			return messages
		}
	}

	compacted := append([]anthropic.BetaMessageParam(nil), messages[boundary:]...)
	compacted = prependSummary(compacted, summary)
	res.Compactions++

	logrus.WithFields(logrus.Fields{
		"replaced":  boundary,
		"kept":      len(messages) - boundary,
		"summary":   len(summary),
		"remaining": len(compacted),
	}).Info("afk harness: compacted conversation")
	return compacted
}

// prependSummary attaches the summary to the first kept message as an extra
// text block, mirroring how the log projects a compaction on reload. The two
// have to agree: one is what this run keeps using, the other is what the next
// turn is rebuilt from.
func prependSummary(msgs []anthropic.BetaMessageParam, summary string) []anthropic.BetaMessageParam {
	if len(msgs) == 0 {
		return msgs
	}
	first := msgs[0]
	content := make([]anthropic.BetaContentBlockParamUnion, 0, len(first.Content)+1)
	content = append(content, anthropic.NewBetaTextBlock(summary))
	content = append(content, first.Content...)
	first.Content = content
	msgs[0] = first
	return msgs
}

// close finishes an interrupted run: it restores the alternating-role invariant
// if the transcript ends on a user turn, then flushes whatever is still pending.
func (h *Harness) close(messages []anthropic.BetaMessageParam, pending []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	if n := len(messages); n > 0 && messages[n-1].Role == anthropic.BetaMessageParamRoleUser {
		note := anthropic.BetaMessageParam{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(interruptedNote)},
		}
		messages = append(messages, note)
		pending = append(pending, note)
	}
	h.checkpoint(&pending)
	return messages
}

// checkpoint flushes pending messages to the log and clears the slice. A write
// failure is logged, not returned: the run's work is real whether or not it was
// recorded, and failing the turn would throw away tool side effects that already
// happened.
func (h *Harness) checkpoint(pending *[]anthropic.BetaMessageParam) {
	if h.log == nil || len(*pending) == 0 {
		*pending = nil
		return
	}
	if err := h.log.Append(*pending...); err != nil {
		logrus.WithError(err).WithField("messages", len(*pending)).
			Error("afk harness: checkpoint failed, conversation may be incomplete on disk")
	}
	*pending = nil
}
