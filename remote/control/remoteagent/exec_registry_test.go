package remoteagent

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSteerable struct {
	accept bool
	got    []string
}

func (f *fakeSteerable) Steer(text string) bool {
	f.got = append(f.got, text)
	return f.accept
}

func TestExecutionRegistry_BeginRejectsSecondRun(t *testing.T) {
	r := newExecutionRegistry()
	require.NoError(t, r.begin("chat", func() {}))

	err := r.begin("chat", func() {})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errExecutionBusy))
}

// A busy chat whose execution is steerable takes the message instead of
// rejecting it — this is the routing decision that turns "session busy" into a
// follow-up.
func TestExecutionRegistry_SteerDeliversToRunningExecution(t *testing.T) {
	r := newExecutionRegistry()
	require.NoError(t, r.begin("chat", func() {}))

	agent := &fakeSteerable{accept: true}
	r.setSteerable("chat", agent)

	assert.True(t, r.steer("chat", "also check the logs"))
	assert.Equal(t, []string{"also check the logs"}, agent.got)
}

// Not every execution can be steered (@cc drives a subprocess). Those must fall
// through to the busy path rather than silently swallowing the message.
func TestExecutionRegistry_SteerFalseWhenNothingRegistered(t *testing.T) {
	r := newExecutionRegistry()
	require.NoError(t, r.begin("chat", func() {}))

	assert.False(t, r.steer("chat", "hello"), "a busy-but-unsteerable chat must not claim the message")
	assert.False(t, r.steer("other-chat", "hello"), "an idle chat has nothing to steer")
}

// An execution that declines the message (its run just ended) must report that,
// so the caller starts a fresh turn instead of dropping it.
func TestExecutionRegistry_SteerRespectsRefusal(t *testing.T) {
	r := newExecutionRegistry()
	require.NoError(t, r.begin("chat", func() {}))
	r.setSteerable("chat", &fakeSteerable{accept: false})

	assert.False(t, r.steer("chat", "too late"))
}

// setSteerable on an idle chat must not leave a dangling entry that a later,
// unrelated run would inherit.
func TestExecutionRegistry_SetSteerableIgnoredWhenNotRunning(t *testing.T) {
	r := newExecutionRegistry()
	r.setSteerable("chat", &fakeSteerable{accept: true})
	assert.False(t, r.steer("chat", "hi"), "no run was registered, so nothing should be steerable")
}

func TestExecutionRegistry_EndClearsSteerable(t *testing.T) {
	r := newExecutionRegistry()
	require.NoError(t, r.begin("chat", func() {}))
	r.setSteerable("chat", &fakeSteerable{accept: true})

	r.end("chat")
	assert.False(t, r.steer("chat", "after the run"), "a finished run must not keep taking messages")
	require.NoError(t, r.begin("chat", func() {}), "the chat should be free to run again")
}

func TestExecutionRegistry_CancelClearsSteerable(t *testing.T) {
	r := newExecutionRegistry()
	var cancelled bool
	require.NoError(t, r.begin("chat", func() { cancelled = true }))
	r.setSteerable("chat", &fakeSteerable{accept: true})

	assert.True(t, r.cancel("chat"))
	assert.True(t, cancelled)
	assert.False(t, r.steer("chat", "after /stop"), "a stopped run must not keep taking messages")
	assert.False(t, r.cancel("chat"), "a second /stop has nothing to stop")
}
