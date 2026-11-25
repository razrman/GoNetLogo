package auth

import (
	"context"
	"fmt"
	"sync"
)

// ErrDisabled is returned when the active provider does not accept
// sign-up or sign-in, which is the case for provider none.
var ErrDisabled = fmt.Errorf("authentication is not enabled")

// ErrAnonymous means the request has no signed-in user.
var ErrAnonymous = fmt.Errorf("not signed in")

// AuthProvider is one way to prove who a person is. Logout and session
// verification sit here so HTTP middleware never mentions a concrete
// provider. A redirect-based provider can return ErrDisabled from
// Signup and Login until pass 2 wires its callback.
type AuthProvider interface {
	Name() string
	Signup(ctx context.Context, req SignupRequest) (Identity, error)
	Login(ctx context.Context, req LoginRequest) (Identity, error)
	Logout(ctx context.Context, sessionID string) error
	Verify(ctx context.Context, sessionID string) (Identity, error)
}

// Factory builds a provider from its config block.
type Factory func(cfg Config, sessions *SessionManager) (AuthProvider, error)

var (
	registryMu sync.Mutex
	registry   = map[string]Factory{}
)

// Register adds a provider under the name used in the config file.
// A second registration of the same name replaces the first.
func Register(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[name] = factory
}

// NewProvider builds the provider named in the config.
func NewProvider(cfg Config, sessions *SessionManager) (AuthProvider, error) {
	registryMu.Lock()
	factory, ok := registry[cfg.Provider]
	registryMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("auth provider %q is not registered", cfg.Provider)
	}
	return factory(cfg, sessions)
}

func init() {
	Register("none", func(Config, *SessionManager) (AuthProvider, error) {
		return noneProvider{}, nil
	})
	Register("local", newLocalProvider)
}

type noneProvider struct{}

func (noneProvider) Name() string { return "none" }

func (noneProvider) Signup(context.Context, SignupRequest) (Identity, error) {
	return Identity{}, ErrDisabled
}

func (noneProvider) Login(context.Context, LoginRequest) (Identity, error) {
	return Identity{}, ErrDisabled
}

func (noneProvider) Logout(context.Context, string) error { return nil }

func (noneProvider) Verify(context.Context, string) (Identity, error) {
	return Identity{}, ErrAnonymous
}
