// Package config reads server settings from environment variables that use
// Laravel Reverb's names, plus an optional JSON file with several apps.
package config

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/vortechron/reverb-go/internal/apps"
)

// Scaling configures the optional Redis pub/sub between nodes.
type Scaling struct {
	Enabled  bool
	Channel  string
	Addr     string
	Password string
	DB       int
	// Secret signs every bus message. Nodes drop messages without a valid signature.
	Secret string
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
		Port:           r.port("REVERB_SERVER_PORT", 8080),
		Path:           strings.TrimRight(r.str("REVERB_SERVER_PATH", ""), "/"),
		MaxRequestSize: int64(r.positive("REVERB_MAX_REQUEST_SIZE", 10000)),
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
		Secret:   r.str("REVERB_SCALING_SECRET", ""),
	}
	if r.err == nil && s.Enabled && s.Secret == "" {
		return Scaling{}, errors.New("config: REVERB_SCALING_SECRET is required when scaling is enabled")
	}
	return s, r.err
}

// reader collects the first parse error so Load stays short.
type reader struct {
	get func(string) string
	err error
}

func (r *reader) fail(err error) {
	if r.err == nil {
		r.err = err
	}
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
	if err != nil {
		r.fail(fmt.Errorf("config: %s must be a number: %w", key, err))
	}
	return n
}

// positive reads a limit. When the variable is set it must be 1 or more.
func (r *reader) positive(key string, def int) int {
	n := r.integer(key, def)
	if n < 1 {
		r.fail(fmt.Errorf("config: %s must be 1 or more, got %d", key, n))
	}
	return n
}

// nonNegative reads a limit where 0 means "no limit".
func (r *reader) nonNegative(key string, def int) int {
	n := r.integer(key, def)
	if n < 0 {
		r.fail(fmt.Errorf("config: %s must not be negative, got %d", key, n))
	}
	return n
}

func (r *reader) port(key string, def int) int {
	n := r.integer(key, def)
	if n < 0 || n > 65535 {
		r.fail(fmt.Errorf("config: %s must be between 0 and 65535, got %d", key, n))
	}
	return n
}

func (r *reader) boolean(key string, def bool) bool {
	switch strings.ToLower(r.get(key)) {
	case "":
		return def
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	r.fail(fmt.Errorf("config: %s must be true or false", key))
	return def
}
