package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var dummyPasswordHash, _ = bcrypt.GenerateFromPassword([]byte("not-a-real-password"), bcrypt.DefaultCost)

type localProvider struct {
	users      UserStore
	sessions   *SessionManager
	admins     map[string]bool
	minPass    int
	defaultMax int
}

func newLocalProvider(cfg Config, sessions *SessionManager) (AuthProvider, error) {
	if sessions == nil {
		return nil, fmt.Errorf("local provider needs a session manager")
	}
	store, err := OpenFileStore(cfg.UsersFile)
	if err != nil {
		return nil, err
	}
	if err := store.PromoteEmails(cfg.InitialAdminEmails); err != nil {
		return nil, err
	}
	admins := map[string]bool{}
	for _, email := range cfg.InitialAdminEmails {
		admins[email] = true
	}
	max := cfg.DefaultMaxSessions
	if max < MinUserSessions {
		max = DefaultMaxSessions
	}
	return &localProvider{users: store, sessions: sessions, admins: admins, minPass: cfg.MinPasswordLength, defaultMax: max}, nil
}

func (p *localProvider) Name() string { return "local" }

func (p *localProvider) Users() UserStore { return p.users }

func (p *localProvider) Signup(ctx context.Context, req SignupRequest) (Identity, error) {
	return p.create(ctx, req, RoleUser, true)
}

// CreateUser is the admin form. The role comes from the form, not from
// the initial-admin email list.
func (p *localProvider) CreateUser(ctx context.Context, req SignupRequest, role string) (Identity, error) {
	return p.create(ctx, req, role, false)
}

func (p *localProvider) sessionLimit(user User) int {
	if user.MaxSessions >= MinUserSessions && user.MaxSessions <= MaxUserSessions {
		return user.MaxSessions
	}
	if p.defaultMax >= MinUserSessions {
		return p.defaultMax
	}
	return DefaultMaxSessions
}

func (p *localProvider) create(_ context.Context, req SignupRequest, role string, promoteListed bool) (Identity, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if !validEmail(email) {
		return Identity{}, fmt.Errorf("enter an email address")
	}
	if len(req.Password) < p.minPass {
		return Identity{}, fmt.Errorf("password must be at least %d characters", p.minPass)
	}
	if len(req.Password) > 200 {
		return Identity{}, fmt.Errorf("password is too long")
	}
	name := strings.TrimSpace(req.DisplayName)
	if name == "" {
		name = strings.Split(email, "@")[0]
	}
	if len(name) > 80 {
		return Identity{}, fmt.Errorf("display name is too long")
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if promoteListed && p.admins[email] {
		role = RoleAdmin
	}
	if role != RoleUser && role != RoleAdmin {
		return Identity{}, fmt.Errorf("role must be user or admin")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return Identity{}, err
	}
	id, err := randomID()
	if err != nil {
		return Identity{}, err
	}
	user := User{
		ID:           id,
		Provider:     "local",
		Subject:      email,
		Email:        email,
		DisplayName:  name,
		PasswordHash: string(hash),
		Role:         role,
		MaxSessions:  p.sessionLimit(User{}),
		Created:      time.Now().UTC(),
	}
	if err := p.users.Create(user); err != nil {
		return Identity{}, err
	}
	return p.identityOf(user), nil
}

func (p *localProvider) Login(_ context.Context, req LoginRequest) (Identity, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	user, ok, err := p.users.ByEmail(email)
	if err != nil {
		return Identity{}, err
	}
	if !ok {
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash, []byte(req.Password))
		return Identity{}, fmt.Errorf("email or password is wrong")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return Identity{}, fmt.Errorf("email or password is wrong")
	}
	if user.Disabled {
		return Identity{}, fmt.Errorf("this account is disabled")
	}
	user.LastLogin = time.Now().UTC()
	if err := p.users.Update(user); err != nil {
		return Identity{}, err
	}
	return p.identityOf(user), nil
}

func (p *localProvider) Logout(_ context.Context, sessionID string) error {
	if sessionID != "" {
		p.sessions.Revoke(sessionID, "")
	}
	return nil
}

func (p *localProvider) Verify(_ context.Context, sessionID string) (Identity, error) {
	item, ok := p.sessions.Lookup(sessionID)
	if !ok {
		return Identity{}, ErrAnonymous
	}
	user, ok, err := p.users.ByID(item.UserID)
	if err != nil {
		return Identity{}, err
	}
	if !ok || user.Disabled {
		p.sessions.Revoke(sessionID, "")
		return Identity{}, ErrAnonymous
	}
	return p.identityOf(user), nil
}

func (p *localProvider) identityOf(user User) Identity {
	return Identity{
		UserID:      user.ID,
		Provider:    user.Provider,
		Subject:     user.Subject,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		MaxSessions: p.sessionLimit(user),
	}
}

func validEmail(email string) bool {
	if len(email) < 3 || len(email) > 254 || strings.ContainsAny(email, " \t") {
		return false
	}
	at := strings.Index(email, "@")
	return at > 0 && at < len(email)-1 && strings.Contains(email[at+1:], ".")
}
