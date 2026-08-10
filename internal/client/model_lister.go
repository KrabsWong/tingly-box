package client

import "context"

// IsModelsEndpointNotSupported checks if an error is ErrModelsEndpointNotSupported
func IsModelsEndpointNotSupported(err error) bool {
	_, ok := err.(*ErrModelsEndpointNotSupported)
	return ok
}

// ModelLister defines the interface for fetching model lists from provider APIs
type ModelLister interface {
	// ListModels returns the list of available models from the provider API,
	// together with the raw upstream payload (Raw) when the implementation has
	// it in hand. Raw is persisted alongside Models so the genuine provider
	// response is available for development triage and data accumulation,
	// mirroring how provider_usage stores raw_response. It may be nil for
	// clients that short-circuit (e.g. cloud-credential providers) or have no
	// bytes to return.
	// Returns ErrModelsEndpointNotSupported if the provider does not support the
	// models endpoint.
	ListModels(ctx context.Context) (*ModelListResult, error)
	Close() error
}

// ModelListResult carries a fetched model list together with the raw upstream
// payload. Raw is any value the client already holds (an SDK response struct, a
// response body, …); the caller marshals it to JSON when persisting.
type ModelListResult struct {
	Models []string
	Raw    any
}
