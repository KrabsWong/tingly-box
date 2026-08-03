package remoteagent

import "strings"

// Output format constants for bot messages
// Centralized for easy customization and i18n support
const (
	// Icons
	IconProject    = "📁" // Project/folder
	IconAgentTB    = "🎯" // Tingly-Box agent (@tb)
	IconAgentCC    = "💬" // Claude Code agent (@cc)
	IconDone       = "✅" // Task completed
	IconError      = "❌" // Error
	IconProcess    = "⏳" // Processing
	IconTool       = "🔧" // Tool call
	IconToolResult = "↳" // Tool result
	IconThinking   = "💭" // Model reasoning (not an answer)
	IconSteer      = "↪" // Follow-up folded into the running task
)

// Agent display names
const (
	AgentNameTB        = "@tb"        // Tingly-Box short name
	AgentNameCC        = "@cc"        // Claude Code short name
	AgentNameTinglyBox = "tingly-box" // mirrors db.DefaultChatAgent; the single source of truth lives in internal/data/db, kept here as a literal so this package does not import db
	AgentNameClaude    = "claude"
)

// SeparatorLine visually splits a message body from its footer.
const SeparatorLine = "───────────────"

// Status messages
const (
	MsgProcessing     = "Processing..."
	MsgTaskDone       = "Task done"
	MsgContinueOrHelp = "Continue or /help."
)

// GetAgentIcon returns the icon for an agent type
func GetAgentIcon(agentType string) string {
	switch agentType {
	case AgentNameTinglyBox, AgentNameTB:
		return IconAgentTB
	default:
		return IconAgentCC
	}
}

// GetAgentDisplayName returns the short display name for an agent type
func GetAgentDisplayName(agentType string) string {
	switch agentType {
	case AgentNameTinglyBox:
		return AgentNameTB
	case AgentNameClaude, "claude-code":
		return AgentNameCC
	default:
		return agentType
	}
}

// BuildFooter creates a compact footer line with agent and path info.
// Format: separator + agent line + path line. Either part is omitted when
// the corresponding value is empty, and an empty footer is returned when
// both are empty so callers don't print a stray separator.
func BuildFooter(agentType, projectPath string) string {
	var b strings.Builder
	if agentType != "" {
		b.WriteString("\n")
		b.WriteString(GetAgentIcon(agentType))
		b.WriteString(" ")
		b.WriteString(GetAgentDisplayName(agentType))
	}
	if projectPath != "" {
		b.WriteString("\n")
		b.WriteString(IconProject)
		b.WriteString(" ")
		b.WriteString(ShortenPath(projectPath))
	}
	if b.Len() == 0 {
		return ""
	}
	return "\n" + SeparatorLine + b.String()
}
