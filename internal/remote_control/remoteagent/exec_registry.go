package remoteagent

import (
	"context"
	"errors"
	"sync"
)

// errExecutionBusy is returned by executionRegistry.begin when the chat
// already has a running execution. Callers detect it with errors.Is to render
// the guided "Session Busy" message instead of a raw error dump.
var errExecutionBusy = errors.New("another execution is already in progress for this chat. Please wait for it to complete or use /stop to cancel it")

// executionRegistry tracks the one running execution per chat and owns the
// cancel functions. It is the single mechanism behind both the duplicate-
// execution guard (begin) and /stop (cancel) — the two used to be separate
// map mutations with divergent locking discipline.
type executionRegistry struct {
	mu      sync.Mutex
	running map[string]*execution
}

// execution is one chat's in-flight run. Keeping cancel and steerable on one
// value rather than in two maps is what makes "steerable implies running" hold
// structurally: there is one key to add and one to delete, so the two can never
// be left disagreeing about whether the chat is busy.
type execution struct {
	cancel    context.CancelFunc
	steerable Steerable
}

// Steerable is a running execution that can take a message mid-run instead of
// making the user wait for it to finish.
type Steerable interface {
	Steer(text string) bool
}

func newExecutionRegistry() *executionRegistry {
	return &executionRegistry{running: make(map[string]*execution)}
}

// setSteerable marks the chat's running execution as able to accept mid-run
// messages. Executors call this once their agent exists, which is necessarily
// after begin — until then the chat is busy but not yet steerable, and a
// message arriving in that window falls back to the busy path.
func (r *executionRegistry) setSteerable(chatID string, s Steerable) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e, running := r.running[chatID]; running {
		e.steerable = s
	}
}

// steer hands text to the chat's running execution, reporting whether it was
// taken. A chat that is busy with something unsteerable returns false, and the
// caller falls back to telling the user it is busy.
func (r *executionRegistry) steer(chatID, text string) bool {
	r.mu.Lock()
	e, ok := r.running[chatID]
	var s Steerable
	if ok {
		s = e.steerable
	}
	r.mu.Unlock()
	if s == nil {
		return false
	}
	return s.Steer(text)
}

// begin registers cancel as the chat's running execution. It fails with
// errExecutionBusy when one is already registered — check-and-set happens
// atomically so an execution can never trip over its own entry.
func (r *executionRegistry) begin(chatID string, cancel context.CancelFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.running[chatID]; exists {
		return errExecutionBusy
	}
	r.running[chatID] = &execution{cancel: cancel}
	return nil
}

// end removes the chat's entry without cancelling. Call when the execution
// finishes; a no-op if /stop already removed it.
func (r *executionRegistry) end(chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.running, chatID)
}

// cancel stops the chat's running execution, reporting whether one was
// running. The entry is removed here so a second /stop reports "nothing to
// stop" instead of double-cancelling.
func (r *executionRegistry) cancel(chatID string) bool {
	r.mu.Lock()
	e, exists := r.running[chatID]
	delete(r.running, chatID)
	r.mu.Unlock()

	if exists && e.cancel != nil {
		e.cancel()
		return true
	}
	return false
}
