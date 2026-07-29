package afk

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
)

// DefaultCompactAtTokens is the prompt size at which a run compacts before its
// next model call.
//
// The number is measured, not guessed: every response reports how many input
// tokens the request actually cost, so the trigger reads the real prompt size
// rather than estimating from character counts. 100k leaves useful headroom
// under the 200k window that is the floor for models worth pointing @tb at,
// while being high enough that ordinary conversations never reach it.
const DefaultCompactAtTokens = 100_000

// compactKeepMessages is how many recent messages compaction tries to keep
// verbatim. Recent turns are where the work is; older ones are what a summary
// can stand in for.
const compactKeepMessages = 12

// summarySystemPrompt instructs the summarizing call. It asks for the things a
// resumed conversation actually needs — decisions, state, open threads — rather
// than a readable recap, because the reader is the model itself.
const summarySystemPrompt = `You are compacting a conversation so it can continue within a smaller context.
Write a dense summary of the exchange below, for the assistant that will continue it.
Preserve: what the user asked for and why, decisions and conclusions reached, files and
directories touched, commands run and their outcomes, current working state, and anything
still outstanding. Drop pleasantries and redundant tool output. Do not address the user;
write it as notes to yourself. Be complete over brief — omitted state cannot be recovered.`

const summaryPrefix = "[Summary of the earlier part of this conversation]\n\n"

// compactionBoundary picks how many leading messages may be replaced by a
// summary, or 0 when there is no safe place to cut.
//
// The cut must land on a genuine user turn — one that opens an exchange rather
// than carrying tool results. Cutting anywhere else orphans a tool_result from
// the tool_use that requested it, which the API rejects outright, and it is
// also where the summary text gets attached, which only makes sense on a user
// turn.
//
// It searches forward from the ideal boundary so compaction keeps at most
// keepLast-ish messages; if the whole tail is one long tool loop with no clean
// turn boundary, it gives up rather than cutting somewhere unsafe.
func compactionBoundary(msgs []anthropic.BetaMessageParam, keepLast int) int {
	if len(msgs) <= keepLast {
		return 0
	}
	for i := len(msgs) - keepLast; i < len(msgs); i++ {
		if isConversationTurnStart(msgs[i]) {
			return i
		}
	}
	return 0
}

// isConversationTurnStart reports whether m opens an exchange: a user message
// that carries no tool results.
func isConversationTurnStart(m anthropic.BetaMessageParam) bool {
	if m.Role != anthropic.BetaMessageParamRoleUser {
		return false
	}
	for _, b := range m.Content {
		if b.OfToolResult != nil {
			return false
		}
	}
	return true
}

// renderForSummary flattens messages into the transcript handed to the
// summarizing model. Tool inputs and results are included in outline form —
// what ran and what came back is exactly the state a resumed conversation needs
// — but truncated, since re-reading a full build log to summarize it would cost
// as much as the context we are trying to reclaim.
func renderForSummary(msgs []anthropic.BetaMessageParam) string {
	var b strings.Builder
	for _, m := range msgs {
		role := "User"
		if m.Role == anthropic.BetaMessageParamRoleAssistant {
			role = "Assistant"
		}
		for _, blk := range m.Content {
			switch {
			case blk.OfText != nil:
				fmt.Fprintf(&b, "%s: %s\n", role, blk.OfText.Text)
			case blk.OfToolUse != nil:
				fmt.Fprintf(&b, "%s called %s(%s)\n", role, blk.OfToolUse.Name,
					Truncate(string(mustJSON(blk.OfToolUse.Input)), TruncateOptions{MaxBytes: 500, MaxLines: 8}).Text)
			case blk.OfToolResult != nil:
				fmt.Fprintf(&b, "Tool result: %s\n",
					Truncate(toolResultText(blk.OfToolResult), TruncateOptions{MaxBytes: 1000, MaxLines: 20, KeepTail: true}).Text)
			}
		}
	}
	return b.String()
}

func toolResultText(r *anthropic.BetaToolResultBlockParam) string {
	var b strings.Builder
	for _, c := range r.Content {
		if c.OfText != nil {
			b.WriteString(c.OfText.Text)
		}
	}
	return b.String()
}

func mustJSON(v any) []byte {
	switch t := v.(type) {
	case []byte:
		return t
	case string:
		return []byte(t)
	default:
		return []byte(fmt.Sprint(v))
	}
}

// Summarize condenses messages into a single block of text using the engine's
// model, with no tools and no conversation history of its own.
func (e *Engine) Summarize(ctx context.Context, msgs []anthropic.BetaMessageParam) (string, error) {
	transcript := renderForSummary(msgs)
	if strings.TrimSpace(transcript) == "" {
		return "", fmt.Errorf("nothing to summarize")
	}

	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(e.model),
		MaxTokens: 4096,
		System:    []anthropic.BetaTextBlockParam{{Text: summarySystemPrompt}},
		Messages: []anthropic.BetaMessageParam{
			anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(transcript)),
		},
	}

	stream := e.client.Beta.Messages.NewStreaming(ctx, params)
	msg := anthropic.BetaMessage{}
	for stream.Next() {
		if err := msg.Accumulate(stream.Current()); err != nil {
			return "", fmt.Errorf("accumulate summary stream: %w", err)
		}
	}
	if err := stream.Err(); err != nil {
		return "", fmt.Errorf("summary stream error: %w", err)
	}

	var out strings.Builder
	for _, block := range msg.Content {
		if block.Type == "text" {
			out.WriteString(block.Text)
		}
	}
	summary := strings.TrimSpace(out.String())
	if summary == "" {
		return "", fmt.Errorf("summary was empty")
	}

	logrus.WithFields(logrus.Fields{
		"summarized_msgs": len(msgs),
		"summary_len":     len(summary),
		"input_tokens":    msg.Usage.InputTokens,
		"output_tokens":   msg.Usage.OutputTokens,
	}).Info("afk: compacted conversation prefix")

	return summaryPrefix + summary, nil
}
