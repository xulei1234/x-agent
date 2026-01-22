package cred

import "context"

// Credentials per rpc call with uuid in metadata
type Credentials struct {
	UUID string
}

// GetRequestMetadata impliment
func (c *Credentials) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	return map[string]string{
		"uuid": c.UUID,
	}, nil
}

// RequireTransportSecurity impliment
func (c *Credentials) RequireTransportSecurity() bool {
	return false
}
