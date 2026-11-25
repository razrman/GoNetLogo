// Package auth is the web sign-in layer. The active provider is chosen
// from config. Provider "none" leaves the site anonymous.
package auth

import "context"

// Role is stored by GoNetLogo, not by the identity provider.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

// Identity is the signed-in person, independent of which provider
// authenticated them.
type Identity struct {
	UserID      string `json:"userId"`
	Provider    string `json:"provider"`
	Subject     string `json:"subject"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
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
