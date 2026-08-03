// Package control is the host-facing assembly layer of the remote
// subsystem. Where remote/control/{bot,remoteagent,...} hold the runtime
// pieces, this file collapses the duplicated construction the two host
// entry points (the managed server path and the standalone CLI path) both
// used to spell out verbatim: the session manager and the agent service.
//
// Keeping that core in one place is what makes "remote is a self-contained
// sub-service" true in code — a host wires it with one call instead of
// re-deriving the session/agent defaults each time.
package control

import (
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// Core bundles the two long-lived components every remote host entry point
// constructs: a session.Manager (transcript + lifecycle) and the agentboot
// AgentService that runs the Claude agent. Both are wired with the same
// defaults regardless of whether the host is the managed server or the
// standalone CLI, so they are built once here.
type Core struct {
	Session  *session.Manager
	Agent    *agentboot.AgentService
}

// NewCore builds the shared session manager and agent service from a session
// store. The session/agent defaults (30-minute timeout, 7-day retention,
// 30-minute agent execution timeout) are the sub-service's own contract —
// callers must not need to know them.
func NewCore(sessionStore session.SessionStore) (*Core, error) {
	if sessionStore == nil {
		return nil, fmt.Errorf("remote session store is nil")
	}
	sessionMgr := session.NewManager(session.Config{
		Timeout:          30 * time.Minute,
		MessageRetention: 7 * 24 * time.Hour,
	}, sessionStore)

	agentBootConfig := agentboot.DefaultConfig()
	agentBootConfig.DefaultExecutionTimeout = 30 * time.Minute
	agentService, err := claude.NewService(agentBootConfig)
	if err != nil {
		return nil, fmt.Errorf("create agent service: %w", err)
	}

	return &Core{Session: sessionMgr, Agent: agentService}, nil
}
