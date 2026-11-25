package auth

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// EnvConfig is the path of the auth file. When it is set, a missing or
// invalid file stops the process. When it is unset, config/auth.yaml is
// used if that file exists, and otherwise the server stays anonymous.
const EnvConfig = "GONETLOGO_AUTH_CONFIG"

// DefaultPath is the file a checkout loads when EnvConfig is unset.
const DefaultPath = "config/auth.yaml"

// Config is the validated auth file.
type Config struct {
	Provider           string
	RequireLogin       bool
	SessionLifetime    time.Duration
	Cookie             CookieConfig
	UsersFile          string
	InitialAdminEmails []string
	MinPasswordLength  int
	DefaultMaxSessions int
	// RawProviders keeps every providers: block, including ones that are
	// only documented. A future plugin reads its own block by name.
	RawProviders map[string]map[string]any
}

// CookieConfig is the signed-in session cookie.
type CookieConfig struct {
	Name     string
	Secure   bool
	SameSite string
	Path     string
}

type fileConfig struct {
	Provider           string                    `yaml:"provider"`
	RequireLogin       bool                      `yaml:"require_login"`
	SessionLifetime    string                    `yaml:"session_lifetime"`
	Cookie             fileCookie                `yaml:"cookie"`
	UsersFile          string                    `yaml:"users_file"`
	InitialAdminEmails []string                  `yaml:"initial_admin_emails"`
	DefaultMaxSessions *int                      `yaml:"default_max_sessions"`
	Providers          map[string]map[string]any `yaml:"providers"`
}

type fileCookie struct {
	Name     string `yaml:"name"`
	Secure   bool   `yaml:"secure"`
	SameSite string `yaml:"same_site"`
	Path     string `yaml:"path"`
}

// DefaultConfig is anonymous access, used when no config file is present.
func DefaultConfig() Config {
	return Config{
		Provider:           "none",
		SessionLifetime:    168 * time.Hour,
		Cookie:             CookieConfig{Name: "gonetlogo_auth", SameSite: "lax", Path: "/"},
		UsersFile:          "data/users.json",
		MinPasswordLength:  8,
		DefaultMaxSessions: DefaultMaxSessions,
		RawProviders:       map[string]map[string]any{},
	}
}

// Load reads the auth file selected by the environment.
func Load(getenv func(string) string) (Config, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	path := strings.TrimSpace(getenv(EnvConfig))
	if path != "" {
		return LoadFile(path)
	}
	if _, err := os.Stat(DefaultPath); err == nil {
		return LoadFile(DefaultPath)
	} else if !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("auth config: %w", err)
	}
	return DefaultConfig(), nil
}

// LoadFile reads and validates one auth file.
func LoadFile(path string) (Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("auth config %s: %w", path, err)
	}
	cfg, err := Parse(b)
	if err != nil {
		return Config{}, fmt.Errorf("auth config %s: %w", path, err)
	}
	return cfg, nil
}

// Parse validates auth YAML.
func Parse(b []byte) (Config, error) {
	var raw fileConfig
	if err := yaml.Unmarshal(b, &raw); err != nil {
		return Config{}, fmt.Errorf("could not read YAML: %w", err)
	}
	cfg := DefaultConfig()
	cfg.Provider = strings.ToLower(strings.TrimSpace(raw.Provider))
	if cfg.Provider == "" {
		return Config{}, fmt.Errorf("provider is required")
	}
	cfg.RequireLogin = raw.RequireLogin
	if strings.TrimSpace(raw.SessionLifetime) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(raw.SessionLifetime))
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("session_lifetime must be a positive duration such as 168h, got %q", raw.SessionLifetime)
		}
		cfg.SessionLifetime = d
	}
	if name := strings.TrimSpace(raw.Cookie.Name); name != "" {
		cfg.Cookie.Name = name
	}
	cfg.Cookie.Secure = raw.Cookie.Secure
	if site := strings.ToLower(strings.TrimSpace(raw.Cookie.SameSite)); site != "" {
		cfg.Cookie.SameSite = site
	}
	switch cfg.Cookie.SameSite {
	case "lax", "strict", "none":
	default:
		return Config{}, fmt.Errorf("cookie.same_site must be lax, strict, or none, got %q", cfg.Cookie.SameSite)
	}
	if cfg.Cookie.SameSite == "none" && !cfg.Cookie.Secure {
		return Config{}, fmt.Errorf("cookie.same_site none requires cookie.secure true")
	}
	if path := strings.TrimSpace(raw.Cookie.Path); path != "" {
		cfg.Cookie.Path = path
	}
	if !strings.HasPrefix(cfg.Cookie.Path, "/") {
		return Config{}, fmt.Errorf("cookie.path must start with /")
	}
	if file := strings.TrimSpace(raw.UsersFile); file != "" {
		cfg.UsersFile = file
	}
	cfg.RawProviders = raw.Providers
	if cfg.RawProviders == nil {
		cfg.RawProviders = map[string]map[string]any{}
	}
	if cfg.Provider == "local" {
		n, err := passwordLength(cfg.RawProviders["local"])
		if err != nil {
			return Config{}, err
		}
		cfg.MinPasswordLength = n
		if cfg.UsersFile == "" {
			return Config{}, fmt.Errorf("users_file is required for the local provider")
		}
	}
	if raw.DefaultMaxSessions != nil {
		n := *raw.DefaultMaxSessions
		if n < MinUserSessions || n > MaxUserSessions {
			return Config{}, fmt.Errorf("default_max_sessions must be from %d to %d", MinUserSessions, MaxUserSessions)
		}
		cfg.DefaultMaxSessions = n
	}
	if cfg.RequireLogin && cfg.Provider == "none" {
		return Config{}, fmt.Errorf("require_login cannot be true when provider is none")
	}
	for _, email := range raw.InitialAdminEmails {
		email = strings.ToLower(strings.TrimSpace(email))
		if email == "" {
			continue
		}
		if !strings.Contains(email, "@") || strings.ContainsAny(email, " \t") {
			return Config{}, fmt.Errorf("initial_admin_emails contains %q, which is not an email address", email)
		}
		cfg.InitialAdminEmails = append(cfg.InitialAdminEmails, email)
	}
	if _, ok := lookupFactory(cfg.Provider); !ok {
		return Config{}, fmt.Errorf("auth provider %q is not registered (cognito and github are documented for a later pass and are not implemented)", cfg.Provider)
	}
	return cfg, nil
}

func passwordLength(block map[string]any) (int, error) {
	if block == nil {
		return 8, nil
	}
	raw, ok := block["min_password_length"]
	if !ok || raw == nil {
		return 8, nil
	}
	n, ok := asInt(raw)
	if !ok || n < 8 || n > 200 {
		return 0, fmt.Errorf("providers.local.min_password_length must be a number from 8 to 200")
	}
	return n, nil
}

func asInt(raw any) (int, bool) {
	switch n := raw.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func lookupFactory(name string) (Factory, bool) {
	registryMu.Lock()
	defer registryMu.Unlock()
	f, ok := registry[name]
	return f, ok
}
