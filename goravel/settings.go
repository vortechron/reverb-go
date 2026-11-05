package goravel

import (
	"context"
	"fmt"
	"strings"

	"github.com/goravel/framework/contracts/config"

	"github.com/vortechron/reverb-go/broadcast"
)

// Broadcast drivers, named as in Laravel's BROADCAST_CONNECTION.
const (
	DriverReverb = "reverb"
	DriverLog    = "log"
	DriverNull   = "null"
)

const driverKey = "driver"

// Config returns the value for config.Add("broadcasting", ...), shaped like
// Laravel's config/broadcasting.php and read from the same env names. Pass the
// config's Env method:
//
//	config.Add("broadcasting", reverbgo.Config(config.Env))
func Config(env func(key string, def ...any) any) map[string]any {
	return map[string]any{
		"default": env("BROADCAST_CONNECTION", DriverNull),
		"connections": map[string]any{
			DriverReverb: map[string]any{
				driverKey: DriverReverb,
				"key":     env("REVERB_APP_KEY"),
				"secret":  env("REVERB_APP_SECRET"),
				"app_id":  env("REVERB_APP_ID"),
				"options": map[string]any{
					"host":   env("REVERB_HOST"),
					"port":   env("REVERB_PORT", 443),
					"scheme": env("REVERB_SCHEME", "https"),
				},
			},
			DriverLog:  map[string]any{driverKey: DriverLog},
			DriverNull: map[string]any{driverKey: DriverNull},
		},
	}
}

// Settings is the parsed broadcasting config.
type Settings struct {
	Driver      string
	Credentials broadcast.Credentials
	// BaseURL is where the Pusher HTTP API listens, for the reverb driver.
	BaseURL string
}

// SettingsFrom reads the "broadcasting" config. It fails on a bad config so
// boot stops instead of dropping events later.
func SettingsFrom(cfg config.Config) (Settings, error) {
	driver := cfg.GetString("broadcasting.default", DriverNull)
	if driver == "" {
		driver = DriverNull
	}
	switch driver {
	case DriverLog, DriverNull:
		return Settings{Driver: driver, Credentials: credentialsFrom(cfg)}, nil
	case DriverReverb:
		return reverbSettings(cfg)
	}

	return Settings{}, fmt.Errorf("reverb: BROADCAST_CONNECTION %q is not one of reverb, log, null", driver)
}

func credentialsFrom(cfg config.Config) broadcast.Credentials {
	const p = "broadcasting.connections.reverb."

	return broadcast.Credentials{AppID: cfg.GetString(p + "app_id"), Key: cfg.GetString(p + "key"), Secret: cfg.GetString(p + "secret")}
}

func reverbSettings(cfg config.Config) (Settings, error) {
	const p = "broadcasting.connections.reverb."
	creds := credentialsFrom(cfg)
	host := cfg.GetString(p + "options.host")
	if creds.AppID == "" || creds.Key == "" || creds.Secret == "" || host == "" {
		return Settings{}, fmt.Errorf("reverb: REVERB_APP_ID, REVERB_APP_KEY, REVERB_APP_SECRET and REVERB_HOST must be set when BROADCAST_CONNECTION=%s", DriverReverb)
	}
	scheme := strings.ToLower(cfg.GetString(p+"options.scheme", "https"))
	if scheme != "http" && scheme != "https" {
		return Settings{}, fmt.Errorf("reverb: REVERB_SCHEME %q must be http or https", scheme)
	}

	return Settings{
		Driver:      DriverReverb,
		Credentials: creds,
		BaseURL:     fmt.Sprintf("%s://%s:%d", scheme, host, cfg.GetInt(p+"options.port", 443)),
	}, nil
}

// InfoLogger is the part of Goravel's log the log driver uses.
type InfoLogger interface {
	Infof(format string, args ...any)
}

// NewPublisher builds the publisher a driver names.
func NewPublisher(s Settings, log InfoLogger) (Publisher, error) {
	switch s.Driver {
	case DriverReverb:
		return broadcast.NewPublisher(broadcast.Config{Credentials: s.Credentials, BaseURL: s.BaseURL}), nil
	case DriverLog:
		return logPublisher{log: log}, nil
	case DriverNull:
		return nullPublisher{}, nil
	}

	return nil, fmt.Errorf("reverb: unknown broadcast driver %q", s.Driver)
}

type nullPublisher struct{}

func (nullPublisher) Trigger(context.Context, []string, string, any, ...broadcast.Option) error {
	return nil
}

type logPublisher struct{ log InfoLogger }

func (p logPublisher) Trigger(_ context.Context, channels []string, name string, _ any, _ ...broadcast.Option) error {
	if p.log != nil {
		p.log.Infof("broadcast %s to %s", name, strings.Join(channels, ","))
	}

	return nil
}
