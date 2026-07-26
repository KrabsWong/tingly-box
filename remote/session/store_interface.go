package session

// SessionStore defines the interface for session persistence, keeping this
// package independent of where sessions are actually stored
type SessionStore interface {
	// Get retrieves a session by ID
	Get(sessionID string) (*Session, error)

	// Set stores a session
	Set(sessionID string, sess *Session) error

	// Delete removes a session
	Delete(sessionID string) error

	// List returns all sessions
	List() []*Session

	// FindByChatAgentProject finds a session by (chatID, agent, project) tuple
	FindByChatAgentProject(chatID, agent, project string) (*Session, error)

	// ListByChat lists all sessions for a given chat ID
	ListByChat(chatID string) ([]*Session, error)

	// AppendMessage adds one message to a session's transcript. Separate from
	// Set because the transcript is append-only and lives outside the session
	// index — see Transcript for why history is not stored as rows.
	AppendMessage(sessionID string, msg Message) error

	// Messages returns a session's full history, read on demand.
	Messages(sessionID string) ([]Message, error)
}
