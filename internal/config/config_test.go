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
		"REVERB_SCALING_ENABLED": "true", "REVERB_SCALING_CHANNEL": "chan",
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
	for name, m := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := config.Load(env(m)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
