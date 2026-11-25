package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

// LoginSession is one signed-in browser. It is not a model workspace.
type LoginSession struct {
	ID        string    `json:"id"`
	UserID    string    `json:"userId"`
	UserAgent string    `json:"userAgent"`
	Created   time.Time `json:"created"`
	LastSeen  time.Time `json:"lastSeen"`
	Expires   time.Time `json:"expires"`
	Current   bool      `json:"current,omitempty"`
}

// SessionManager keeps login sessions in memory. A process restart
// signs everyone out. The cookie value is the session id.
type SessionManager struct {
	mu       sync.Mutex
	items    map[string]LoginSession
	lifetime time.Duration
	cookie   CookieConfig
}

func NewSessionManager(cfg Config) *SessionManager {
	return &SessionManager{
		items:    map[string]LoginSession{},
		lifetime: cfg.SessionLifetime,
		cookie:   cfg.Cookie,
	}
}

func (m *SessionManager) Start(userID, userAgent string) (LoginSession, error) {
	id, err := randomID()
	if err != nil {
		return LoginSession{}, err
	}
	now := time.Now().UTC()
	item := LoginSession{
		ID:        id,
		UserID:    userID,
		UserAgent: trimAgent(userAgent),
		Created:   now,
		LastSeen:  now,
		Expires:   now.Add(m.lifetime),
	}
	m.mu.Lock()
	m.items[id] = item
	m.mu.Unlock()
	return item, nil
}

func (m *SessionManager) Lookup(id string) (LoginSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok || time.Now().After(item.Expires) {
		if ok {
			delete(m.items, id)
		}
		return LoginSession{}, false
	}
	item.LastSeen = time.Now().UTC()
	m.items[id] = item
	return item, true
}

func (m *SessionManager) ForUser(userID, currentID string) []LoginSession {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	var out []LoginSession
	for id, item := range m.items {
		if item.UserID != userID {
			continue
		}
		if now.After(item.Expires) {
			delete(m.items, id)
			continue
		}
		item.Current = item.ID == currentID
		out = append(out, item)
	}
	return out
}

func (m *SessionManager) Revoke(id, userID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	item, ok := m.items[id]
	if !ok || (userID != "" && item.UserID != userID) {
		return false
	}
	delete(m.items, id)
	return true
}

// RevokeUser drops every login session for a user. allOthers keeps currentID.
func (m *SessionManager) RevokeUser(userID, currentID string, allOthers bool) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for id, item := range m.items {
		if item.UserID != userID {
			continue
		}
		if allOthers && id == currentID {
			continue
		}
		delete(m.items, id)
		n++
	}
	return n
}

func (m *SessionManager) SetCookie(w http.ResponseWriter, id string) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookie.Name,
		Value:    id,
		Path:     m.cookie.Path,
		HttpOnly: true,
		Secure:   m.cookie.Secure,
		SameSite: sameSite(m.cookie.SameSite),
		Expires:  time.Now().Add(m.lifetime),
		MaxAge:   int(m.lifetime.Seconds()),
	})
}

func (m *SessionManager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookie.Name,
		Value:    "",
		Path:     m.cookie.Path,
		HttpOnly: true,
		Secure:   m.cookie.Secure,
		SameSite: sameSite(m.cookie.SameSite),
		MaxAge:   -1,
		Expires:  time.Unix(0, 0),
	})
}

func (m *SessionManager) IDFromRequest(r *http.Request) string {
	c, err := r.Cookie(m.cookie.Name)
	if err != nil {
		return ""
	}
	return c.Value
}

func sameSite(name string) http.SameSite {
	switch name {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func randomID() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func trimAgent(agent string) string {
	if len(agent) > 200 {
		return agent[:200]
	}
	return agent
}
