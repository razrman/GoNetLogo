// Package auth is the web sign-in layer. The active provider is chosen
// from config. Provider "none" leaves the site anonymous.
package auth

import (
	"context"
	"fmt"
)

// Role is stored by GoNetLogo, not by the identity provider.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Session limits an admin can set, and the default when auth.yaml omits one.
const (
	MinUserSessions    = 1
	MaxUserSessions    = 50
	DefaultMaxSessions = 4
)

// SessionLimitError is the GraphQL error when a signed-in user is at their cap.
func SessionLimitError(open, max int) error {
	return fmt.Errorf("you have %d open model sessions, which is your limit of %d. End one on the Account page before starting another", open, max)
}

// Identity is the signed-in person, independent of which provider
// authenticated them.
type Identity struct {
	UserID      string `json:"userId"`
	Provider    string `json:"provider"`
	Subject     string `json:"subject"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	MaxSessions int    `json:"maxSessions,omitempty"`
}

// SignupRequest is the local sign-up form. Other providers may ignore
// the password and use a redirect instead.
type SignupRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

// LoginRequest is the local sign-in form.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type ctxKey struct{}

// WithIdentity stores the signed-in user on a request context.
func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// FromContext returns the signed-in user, if middleware found one.
func FromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	if !ok || id.UserID == "" {
		return Identity{}, false
	}
	return id, true
}
