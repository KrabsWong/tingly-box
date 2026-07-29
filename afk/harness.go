package afk

import (
	"context"

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
}

// NewHarness builds a Harness over an engine. log may be nil.
func NewHarness(e *Engine, log Log) *Harness {
	return &Harness{engine: e, log: log}
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

		messages = append(messages, step.Messages...)
		pending = append(pending, step.Messages...)
		res.Steps++
		res.Usage.Add(step.Usage)
		if step.Text != "" {
			res.FinalText = step.Text
		}

		logrus.WithFields(logrus.Fields{
			"step":       i,
			"turn_text":  len(step.Text),
			"tool_calls": step.ToolCalls,
		}).Debug("afk harness: step complete")

		// Checkpoint: the step is done and its messages are well-formed, so
		// they can be made durable before the next model call is risked.
		h.checkpoint(&pending)

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
