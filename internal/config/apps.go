package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/vortechron/reverb-go/internal/apps"
)

// Defaults. The abuse limits are on by default, which Reverb's rate limiting is not.
const (
	defaultPingInterval    = 60
	defaultActivityTimeout = 30
	defaultMaxMessageSize  = 10000
	defaultMaxChannels     = 100
	defaultMaxMembers      = 100
	defaultMaxChannelData  = 5000
	defaultRateAttempts    = 600
	defaultRateDecay       = 60
)

func loadApps(r *reader) ([]apps.App, error) {
	if path := r.str("REVERB_APPS_FILE", ""); path != "" {
		return appsFromFile(path)
	}
	app := apps.App{
		ID:                 r.str("REVERB_APP_ID", ""),
		Key:                r.str("REVERB_APP_KEY", ""),
		Secret:             r.str("REVERB_APP_SECRET", ""),
		AllowedOrigins:     splitList(r.str("REVERB_ALLOWED_ORIGINS", "*")),
		PingInterval:       seconds(r.positive("REVERB_APP_PING_INTERVAL", defaultPingInterval)),
		ActivityTimeout:    seconds(r.positive("REVERB_APP_ACTIVITY_TIMEOUT", defaultActivityTimeout)),
		MaxConnections:     r.nonNegative("REVERB_APP_MAX_CONNECTIONS", 0),
		MaxMessageSize:     int64(r.positive("REVERB_APP_MAX_MESSAGE_SIZE", defaultMaxMessageSize)),
		MaxChannels:        r.positive("REVERB_APP_MAX_CHANNELS_PER_CONNECTION", defaultMaxChannels),
		MaxPresenceMembers: r.positive("REVERB_APP_MAX_PRESENCE_MEMBERS", defaultMaxMembers),
		MaxChannelDataSize: r.positive("REVERB_APP_MAX_CHANNEL_DATA_SIZE", defaultMaxChannelData),
		RateLimit: apps.RateLimit{
			Enabled:     r.boolean("REVERB_APP_RATE_LIMITING_ENABLED", true),
			MaxAttempts: r.positive("REVERB_APP_RATE_LIMIT_MAX_ATTEMPTS", defaultRateAttempts),
			Decay:       seconds(r.positive("REVERB_APP_RATE_LIMIT_DECAY_SECONDS", defaultRateDecay)),
			Terminate:   r.boolean("REVERB_APP_RATE_LIMIT_TERMINATE", false),
		},
	}
	mode, err := parseMode(r.str("REVERB_APP_ACCEPT_CLIENT_EVENTS_FROM", "members"))
	if err != nil {
		return nil, err
	}
	app.ClientEvents = mode
	if r.err != nil {
		return nil, r.err
	}
	return []apps.App{app}, nil
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

// fileApp is one app in REVERB_APPS_FILE. Missing numbers use the defaults.
type fileApp struct {
	ID                 string   `json:"id"`
	Key                string   `json:"key"`
	Secret             string   `json:"secret"`
	AllowedOrigins     []string `json:"allowed_origins"`
	PingInterval       *int     `json:"ping_interval"`
	ActivityTimeout    *int     `json:"activity_timeout"`
	MaxConnections     int      `json:"max_connections"`
	MaxMessageSize     *int64   `json:"max_message_size"`
	MaxChannels        *int     `json:"max_channels_per_connection"`
	MaxPresenceMembers *int     `json:"max_presence_members"`
	MaxChannelDataSize *int     `json:"max_channel_data_size"`
	ClientEvents       string   `json:"accept_client_events_from"`
	RateLimiting       *struct {
		Enabled     *bool `json:"enabled"`
		MaxAttempts *int  `json:"max_attempts"`
		DecaySecond *int  `json:"decay_seconds"`
		Terminate   bool  `json:"terminate_on_limit"`
	} `json:"rate_limiting"`
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
		MaxConnections: f.MaxConnections, ClientEvents: parsed,
		RateLimit: apps.RateLimit{Enabled: true, MaxAttempts: defaultRateAttempts, Decay: seconds(defaultRateDecay)},
	}
	a.PingInterval = seconds(orDefault(f.PingInterval, defaultPingInterval))
	a.ActivityTimeout = seconds(orDefault(f.ActivityTimeout, defaultActivityTimeout))
	a.MaxMessageSize = int64(orDefault(intPtr(f.MaxMessageSize), defaultMaxMessageSize))
	a.MaxChannels = orDefault(f.MaxChannels, defaultMaxChannels)
	a.MaxPresenceMembers = orDefault(f.MaxPresenceMembers, defaultMaxMembers)
	a.MaxChannelDataSize = orDefault(f.MaxChannelDataSize, defaultMaxChannelData)
	if rl := f.RateLimiting; rl != nil {
		if rl.Enabled != nil {
			a.RateLimit.Enabled = *rl.Enabled
		}
		a.RateLimit.MaxAttempts = orDefault(rl.MaxAttempts, defaultRateAttempts)
		a.RateLimit.Decay = seconds(orDefault(rl.DecaySecond, defaultRateDecay))
		a.RateLimit.Terminate = rl.Terminate
	}
	return a, validateApp(a)
}

func intPtr(p *int64) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

func orDefault(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

// validateApp rejects zero and negative limits for apps that come from a file.
func validateApp(a apps.App) error {
	checks := map[string]bool{
		"ping_interval":               a.PingInterval > 0,
		"activity_timeout":            a.ActivityTimeout > 0,
		"max_message_size":            a.MaxMessageSize > 0,
		"max_channels_per_connection": a.MaxChannels > 0,
		"max_presence_members":        a.MaxPresenceMembers > 0,
		"max_channel_data_size":       a.MaxChannelDataSize > 0,
		"rate_limiting.max_attempts":  a.RateLimit.MaxAttempts > 0,
		"rate_limiting.decay_seconds": a.RateLimit.Decay > 0,
		"max_connections":             a.MaxConnections >= 0,
	}
	for name, ok := range checks {
		if !ok {
			return fmt.Errorf("config: app %q: %s must be 1 or more", a.ID, name)
		}
	}
	return nil
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
