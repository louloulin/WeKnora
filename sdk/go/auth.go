package weknora

import "net/http"

// Authenticator applies the current authentication material to an outgoing
// request. Implementations must be safe for concurrent use because the
// SDK reuses them across all service goroutines.
type Authenticator interface {
	Apply(*http.Request) error
}

type bearerAuth struct{ token string }

func (b *bearerAuth) Apply(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+b.token)
	return nil
}

type apiKeyAuth struct{ key string }

func (a *apiKeyAuth) Apply(req *http.Request) error {
	req.Header.Set("X-API-Key", a.key)
	return nil
}
