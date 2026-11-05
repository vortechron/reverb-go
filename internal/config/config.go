// Package config reads server settings from environment variables that use
// Laravel Reverb's names, plus an optional JSON file with several apps.
package config

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/vortechron/reverb-go/internal/apps"
)

// Scaling configures the optional Redis pub/sub between nodes.
type Scaling struct {
	Enabled  bool
	Channel  string
	Addr     string
	Password string
	DB       int
}

// Config is the full server configuration.
type Config struct {
	Host           string
	Port           int
	Path           string
	MaxRequestSize int64
	Apps           *apps.Registry
	Scaling        Scaling
}

// Addr returns host:port for the listener.
func (c Config) Addr() string { return net.JoinHostPort(c.Host, strconv.Itoa(c.Port)) }

// Load reads configuration through getenv (use os.Getenv in production).
func Load(getenv func(string) string) (Config, error) {
	r := reader{get: getenv}
	cfg := Config{
		Host:           r.str("REVERB_SERVER_HOST", "0.0.0.0"),
		Port:           r.integer("REVERB_SERVER_PORT", 8080),
		Path:           strings.TrimRight(r.str("REVERB_SERVER_PATH", ""), "/"),
		MaxRequestSize: int64(r.integer("REVERB_MAX_REQUEST_SIZE", 10000)),
	}
	list, err := loadApps(&r)
	if err != nil {
		return Config{}, err
	}
	if cfg.Scaling, err = loadScaling(&r); err != nil {
		return Config{}, err
	}
	if r.err != nil {
		return Config{}, r.err
	}
	if cfg.Apps, err = apps.NewRegistry(list); err != nil {
		return Config{}, fmt.Errorf("config: %w", err)
	}
	return cfg, nil
}

func loadScaling(r *reader) (Scaling, error) {
	s := Scaling{
		Enabled:  r.str("REVERB_SCALING_ENABLED", "false") == "true",
		Channel:  r.str("REVERB_SCALING_CHANNEL", "reverb-go"),
		Addr:     net.JoinHostPort(r.str("REDIS_HOST", "127.0.0.1"), r.str("REDIS_PORT", "6379")),
		Password: r.str("REDIS_PASSWORD", ""),
		DB:       r.integer("REDIS_DB", 0),
	}
	return s, r.err
}

func loadApps(r *reader) ([]apps.App, error) {
	if path := r.str("REVERB_APPS_FILE", ""); path != "" {
		return appsFromFile(path)
	}
	app := apps.App{
		ID:              r.str("REVERB_APP_ID", ""),
		Key:             r.str("REVERB_APP_KEY", ""),
		Secret:          r.str("REVERB_APP_SECRET", ""),
		AllowedOrigins:  splitList(r.str("REVERB_ALLOWED_ORIGINS", "*")),
		PingInterval:    seconds(r.integer("REVERB_APP_PING_INTERVAL", 60)),
		ActivityTimeout: seconds(r.integer("REVERB_APP_ACTIVITY_TIMEOUT", 30)),
		MaxConnections:  r.integer("REVERB_APP_MAX_CONNECTIONS", 0),
		MaxMessageSize:  int64(r.integer("REVERB_APP_MAX_MESSAGE_SIZE", 10000)),
	}
	mode, err := parseMode(r.str("REVERB_APP_ACCEPT_CLIENT_EVENTS_FROM", "members"))
	if err != nil {
		return nil, err
	}
	app.ClientEvents = mode
	if r.err != nil {
		return nil, r.err
	}
	if err := validateTimers(app); err != nil {
		return nil, err
	}
	return []apps.App{app}, nil
}

func validateTimers(a apps.App) error {
	if a.ActivityTimeout <= 0 || a.PingInterval <= 0 {
		return fmt.Errorf("config: activity timeout and ping interval must be above zero")
	}
	return nil
}

func parseMode(v string) (apps.ClientEvents, error) {
	switch m := apps.ClientEvents(strings.ToLower(v)); m {
	case apps.ClientEventsMembers, apps.ClientEventsAll, apps.ClientEventsOff:
		return m, nil
	case "false", "off", "":
		return apps.ClientEventsOff, nil
	default:
		return "", fmt.Errorf("config: unknown accept_client_events_from %q", v)
	}
}

type fileApp struct {
	ID              string   `json:"id"`
	Key             string   `json:"key"`
	Secret          string   `json:"secret"`
	AllowedOrigins  []string `json:"allowed_origins"`
	PingInterval    *int     `json:"ping_interval"`
	ActivityTimeout *int     `json:"activity_timeout"`
	MaxConnections  int      `json:"max_connections"`
	MaxMessageSize  *int64   `json:"max_message_size"`
	ClientEvents    string   `json:"accept_client_events_from"`
}

func appsFromFile(path string) ([]apps.App, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // path is operator supplied configuration.
	if err != nil {
		return nil, fmt.Errorf("config: read apps file: %w", err)
	}
	var doc struct {
		Apps []fileApp `json:"apps"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("config: parse apps file: %w", err)
	}
	out := make([]apps.App, 0, len(doc.Apps))
	for _, f := range doc.Apps {
		a, err := f.toApp()
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, nil
}

func (f fileApp) toApp() (apps.App, error) {
	mode := f.ClientEvents
	if mode == "" {
		mode = "members"
	}
	parsed, err := parseMode(mode)
	if err != nil {
		return apps.App{}, err
	}
	a := apps.App{
		ID: f.ID, Key: f.Key, Secret: f.Secret, AllowedOrigins: f.AllowedOrigins,
		PingInterval: 60 * time.Second, ActivityTimeout: 30 * time.Second,
		MaxConnections: f.MaxConnections, MaxMessageSize: 10000, ClientEvents: parsed,
	}
	if f.PingInterval != nil {
		a.PingInterval = seconds(*f.PingInterval)
	}
	if f.ActivityTimeout != nil {
		a.ActivityTimeout = seconds(*f.ActivityTimeout)
	}
	if f.MaxMessageSize != nil {
		a.MaxMessageSize = *f.MaxMessageSize
	}
	return a, validateTimers(a)
}

func seconds(n int) time.Duration { return time.Duration(n) * time.Second }

func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// reader collects the first parse error so Load stays short.
type reader struct {
	get func(string) string
	err error
}

func (r *reader) str(key, def string) string {
	if v := r.get(key); v != "" {
		return v
	}
	return def
}

func (r *reader) integer(key string, def int) int {
	v := r.get(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil && r.err == nil {
		r.err = fmt.Errorf("config: %s must be a number: %w", key, err)
	}
	return n
}
