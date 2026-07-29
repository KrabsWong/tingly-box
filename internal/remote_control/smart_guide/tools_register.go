package smart_guide

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/afk"
	"github.com/tingly-dev/tingly-box/afk/skill"
)

// BuildTools assembles the Smart Guide toolset for the ReAct engine.
//
// The set mirrors the previous agentscope registration: bash, get_status,
// change_workdir, native read/write/edit, and (when a SendFile callback is
// available) send_file — plus activate_skill when any skills were discovered.
func BuildTools(
	executor *ToolExecutor,
	chatID string,
	getStatusFunc func(chatID string) (*StatusInfo, error),
	updateProjectFunc func(chatID string, projectPath string) error,
	toolCtx *ToolContext,
	skills skill.Skills,
) []afk.Tool {
	tools := []afk.Tool{
		NewBashTool(executor, DefaultBashAllowlist),
		NewGetStatusTool(executor, chatID, getStatusFunc),
		NewChangeDirTool(executor, chatID, updateProjectFunc),
		NewReadFileTool(executor),
		NewWriteFileTool(executor),
		NewEditFileTool(executor),
	}

	if toolCtx != nil && toolCtx.SendFile != nil {
		tools = append(tools, NewSendFileTool(executor, toolCtx))
	}

	// One activator rather than a tool per skill: the skill catalog is carried
	// in the activator's description, so adding a skill costs a few lines of
	// prompt instead of a whole tool definition, and bodies stay out of context
	// until something actually needs them.
	//
	// No config toggle: a skill directory that exists is a skill directory the
	// user meant to use, and an agent with no skills gets no tool.
	if len(skills) > 0 {
		activator, err := skill.NewActivateSkillTool(skills)
		if err != nil {
			// Never fatal — losing skills degrades @tb, it does not break it.
			logrus.WithError(err).Warn("SmartGuide: skill activator not registered")
		} else {
			tools = append(tools, activator)
			logrus.WithField("skills", skills.Names()).Info("SmartGuide: skills available")
		}
	}

	return tools
}
