package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/config"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestLoadSingleAppFromReverbEnvNames(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"REVERB_APP_ID": "42", "REVERB_APP_KEY": "key", "REVERB_APP_SECRET": "sec",
		"REVERB_SERVER_HOST": "127.0.0.1", "REVERB_SERVER_PORT": "6001",
		"REVERB_ALLOWED_ORIGINS": "a.test, *.b.test",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr() != "127.0.0.1:6001" {
		t.Fatalf("addr = %q", cfg.Addr())
	}
	a, ok := cfg.Apps.ByKey("key")
	if !ok || a.ID != "42" || a.Secret != "sec" {
		t.Fatalf("app = %+v", a)
	}
	if a.ActivityTimeout != 30*time.Second || a.PingInterval != 60*time.Second ||
		a.MaxMessageSize != 10000 || a.ClientEvents != apps.ClientEventsMembers {
		t.Fatalf("defaults wrong: %+v", a)
	}
	if len(a.AllowedOrigins) != 2 || a.AllowedOrigins[1] != "*.b.test" {
		t.Fatalf("origins = %v", a.AllowedOrigins)
	}
	if cfg.MaxRequestSize != 10000 || cfg.Scaling.Enabled {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestLoadDefaultsHostAndPort(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s"}))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr() != "0.0.0.0:8080" {
		t.Fatalf("addr = %q", cfg.Addr())
	}
}

func TestLoadReadsAppOptionsAndScaling(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s",
		"REVERB_APP_ACTIVITY_TIMEOUT": "10", "REVERB_APP_PING_INTERVAL": "20",
		"REVERB_APP_MAX_CONNECTIONS": "5", "REVERB_APP_MAX_MESSAGE_SIZE": "500",
		"REVERB_APP_ACCEPT_CLIENT_EVENTS_FROM": "all", "REVERB_SERVER_PATH": "/ws/",
		"REVERB_SCALING_ENABLED": "true", "REVERB_SCALING_CHANNEL": "chan", "REVERB_SCALING_SECRET": "sec",
		"REDIS_HOST": "r.test", "REDIS_PORT": "6380", "REDIS_DB": "13", "REDIS_PASSWORD": "pw",
	}))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := cfg.Apps.ByKey("k")
	if a.ActivityTimeout != 10*time.Second || a.PingInterval != 20*time.Second ||
		a.MaxConnections != 5 || a.MaxMessageSize != 500 || a.ClientEvents != apps.ClientEventsAll {
		t.Fatalf("app = %+v", a)
	}
	if cfg.Path != "/ws" {
		t.Fatalf("path = %q", cfg.Path)
	}
	s := cfg.Scaling
	if !s.Enabled || s.Channel != "chan" || s.Addr != "r.test:6380" || s.DB != 13 || s.Password != "pw" {
		t.Fatalf("scaling = %+v", s)
	}
}

func TestLoadAppsFromFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.json")
	body := `{"apps":[{"id":"1","key":"a","secret":"x","allowed_origins":["o.test"],"accept_client_events_from":"none","activity_timeout":5},
	{"id":"2","key":"b","secret":"y"}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(env(map[string]string{"REVERB_APPS_FILE": path}))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := cfg.Apps.ByKey("a")
	if a.ClientEvents != apps.ClientEventsOff || a.ActivityTimeout != 5*time.Second || a.AllowedOrigins[0] != "o.test" {
		t.Fatalf("a = %+v", a)
	}
	b, _ := cfg.Apps.ByKey("b")
	if b.ActivityTimeout != 30*time.Second {
		t.Fatalf("b must get defaults: %+v", b)
	}
}

func TestLoadRejectsBadConfig(t *testing.T) {
	good := map[string]string{"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s"}
	with := func(k, v string) map[string]string {
		m := map[string]string{}
		for a, b := range good {
			m[a] = b
		}
		m[k] = v
		return m
	}
	cases := map[string]map[string]string{
		"no app":          {},
		"bad port":        with("REVERB_SERVER_PORT", "x"),
		"bad timeout":     with("REVERB_APP_ACTIVITY_TIMEOUT", "-1"),
		"bad client mode": with("REVERB_APP_ACCEPT_CLIENT_EVENTS_FROM", "bogus"),
		"missing file":    {"REVERB_APPS_FILE": "/no/such/file.json"},
		"bad scaling db":  with("REVERB_SCALING_ENABLED", "true"),
	}
	cases["bad scaling db"]["REDIS_DB"] = "x"
	cases["bad scaling db"]["REVERB_SCALING_SECRET"] = "x"
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := config.Load(env(m)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestLoadAbuseLimitDefaults(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s"}))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := cfg.Apps.ByKey("k")
	if a.MaxChannels != 100 || a.MaxPresenceMembers != 100 || a.MaxChannelDataSize != 5000 {
		t.Fatalf("limits = %+v", a)
	}
	rl := a.RateLimit
	if !rl.Enabled || rl.MaxAttempts != 600 || rl.Decay != time.Minute || rl.Terminate {
		t.Fatalf("rate limit = %+v", rl)
	}
}

func TestLoadAbuseLimitOverrides(t *testing.T) {
	cfg, err := config.Load(env(map[string]string{
		"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s",
		"REVERB_APP_MAX_CHANNELS_PER_CONNECTION": "7", "REVERB_APP_MAX_PRESENCE_MEMBERS": "8",
		"REVERB_APP_MAX_CHANNEL_DATA_SIZE": "900", "REVERB_APP_RATE_LIMITING_ENABLED": "false",
		"REVERB_APP_RATE_LIMIT_MAX_ATTEMPTS": "9", "REVERB_APP_RATE_LIMIT_DECAY_SECONDS": "10",
		"REVERB_APP_RATE_LIMIT_TERMINATE": "true",
	}))
	if err != nil {
		t.Fatal(err)
	}
	a, _ := cfg.Apps.ByKey("k")
	rl := a.RateLimit
	if a.MaxChannels != 7 || a.MaxPresenceMembers != 8 || a.MaxChannelDataSize != 900 ||
		rl.Enabled || rl.MaxAttempts != 9 || rl.Decay != 10*time.Second || !rl.Terminate {
		t.Fatalf("app = %+v", a)
	}
}

func TestLoadRejectsNonPositiveLimits(t *testing.T) {
	for _, key := range []string{
		"REVERB_APP_MAX_MESSAGE_SIZE", "REVERB_MAX_REQUEST_SIZE", "REVERB_APP_MAX_CHANNELS_PER_CONNECTION",
		"REVERB_APP_MAX_PRESENCE_MEMBERS", "REVERB_APP_MAX_CHANNEL_DATA_SIZE",
		"REVERB_APP_RATE_LIMIT_MAX_ATTEMPTS", "REVERB_APP_RATE_LIMIT_DECAY_SECONDS",
		"REVERB_APP_PING_INTERVAL", "REVERB_APP_ACTIVITY_TIMEOUT",
	} {
		for _, v := range []string{"0", "-5"} {
			t.Run(key+"="+v, func(t *testing.T) {
				_, err := config.Load(env(map[string]string{
					"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s", key: v,
				}))
				if err == nil {
					t.Fatal("expected an error")
				}
			})
		}
	}
	if _, err := config.Load(env(map[string]string{
		"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s", "REVERB_APP_MAX_CONNECTIONS": "-1",
	})); err == nil {
		t.Fatal("negative max connections must be rejected")
	}
	if _, err := config.Load(env(map[string]string{
		"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s", "REVERB_SERVER_PORT": "70000",
	})); err == nil {
		t.Fatal("port above 65535 must be rejected")
	}
}

func TestFileAppsAreValidatedToo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "apps.json")
	body := `{"apps":[{"id":"1","key":"a","secret":"x","max_message_size":0}]}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(env(map[string]string{"REVERB_APPS_FILE": path})); err == nil {
		t.Fatal("max_message_size 0 must be rejected")
	}
}

func TestScalingNeedsASecret(t *testing.T) {
	base := map[string]string{"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s", "REVERB_SCALING_ENABLED": "true"}
	if _, err := config.Load(env(base)); err == nil {
		t.Fatal("scaling without REVERB_SCALING_SECRET must be rejected")
	}
	base["REVERB_SCALING_SECRET"] = "bus-secret"
	cfg, err := config.Load(env(base))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Scaling.Secret != "bus-secret" {
		t.Fatalf("secret = %q", cfg.Scaling.Secret)
	}
}
