// Package interaction provides platform-agnostic interactive element types:
// inline keyboards (keyboard.go) and rich cards (card.go).
//
// Buttons built here carry their identity as a core.Payload; the keyboard
// types remain as the legacy-compat bridge while call sites migrate to
// core.ActionSet (see imbot/core/action.go).
package interaction

// ActionType represents the type of user action
type ActionType string

const (
	ActionSelect   ActionType = "select"   // User selected an option
	ActionConfirm  ActionType = "confirm"  // User confirmed yes/no
	ActionCancel   ActionType = "cancel"   // User cancelled
	ActionNavigate ActionType = "navigate" // User navigated (prev/next)
	ActionInput    ActionType = "input"    // User provided text input
	ActionCustom   ActionType = "custom"   // Custom action
)

// Interaction represents a platform-agnostic interactive element
type Interaction struct {
	ID      string         // Unique identifier for this interaction
	Type    ActionType     // Type of action
	Label   string         // Display label
	Value   string         // Associated value
	Options []Option       // For select actions
	Meta    map[string]any // Additional data
}

// Option represents a selectable option
type Option struct {
	ID    string // Option ID
	Label string // Display label
	Value string // Associated value
}
