package client

import "context"

// IsModelsEndpointNotSupported checks if an error is ErrModelsEndpointNotSupported
func IsModelsEndpointNotSupported(err error) bool {
	_, ok := err.(*ErrModelsEndpointNotSupported)
	return ok
}

// ModelLister fetches model lists from provider APIs.
type ModelLister interface {
	// ListModels returns parsed model IDs and, when available, the raw upstream
	// payload. Unsupported providers return ErrModelsEndpointNotSupported.
	ListModels(ctx context.Context) (*ModelListResult, error)
	Close() error
}

// ModelListResult carries parsed model IDs and the raw upstream payload.
type ModelListResult struct {
	Models []string
	Raw    any
}
