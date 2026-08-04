package server

import "github.com/tingly-dev/tingly-box/internal/guardrails"

// The exported methods below adapt the protocolserver-owned GuardrailsState
// to the webui.GuardrailsRuntime interface, so the WebUI Management API's
// guardrails admin handler (internal/server.GuardrailsHandler) can drive the
// runtime without root server depending on webui's types. The unexported
// helpers keep the pre-move `s.xxx()` call sites in root lifecycle code
// (server.go, server_flags.go, server_options.go) compiling unchanged.

// CurrentGuardrailsRuntime returns the active guardrails runtime snapshot.
func (s *Server) CurrentGuardrailsRuntime() *guardrails.Guardrails {
	return s.guardrailsState.Current()
}

// SetGuardrailsRuntime swaps in a new guardrails runtime, preserving history
// and credential-cache state carried over from the previous runtime.
func (s *Server) SetGuardrailsRuntime(runtime *guardrails.Guardrails, context string) {
	s.guardrailsState.Set(runtime, context)
}

// GetGuardrailsSupportedScenarios returns the scenarios guardrails can gate.
func (s *Server) GetGuardrailsSupportedScenarios() []string {
	return s.guardrailsState.SupportedScenarios()
}

// RefreshGuardrailsCredentialCacheOrWarn rebuilds the protected-credential
// cache, logging (rather than returning) any failure.
func (s *Server) RefreshGuardrailsCredentialCacheOrWarn(context string) {
	s.guardrailsState.RefreshCredentialCacheOrWarn(context)
}

// Unexported forwarders for root lifecycle code.

func (s *Server) currentGuardrailsRuntime() *guardrails.Guardrails {
	if s == nil {
		return nil
	}
	return s.guardrailsState.Current()
}

func (s *Server) setGuardrailsRuntimeRef(runtime *guardrails.Guardrails) {
	if s == nil {
		return
	}
	s.guardrailsState.SetRef(runtime)
}

func (s *Server) setGuardrailsRuntime(runtime *guardrails.Guardrails, context string) {
	s.guardrailsState.Set(runtime, context)
}

func (s *Server) refreshGuardrailsCredentialCacheOrWarn(context string) {
	s.guardrailsState.RefreshCredentialCacheOrWarn(context)
}

func (s *Server) getGuardrailsSupportedScenarios() []string {
	return s.guardrailsState.SupportedScenarios()
}
