package goravel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	goravelconfig "github.com/goravel/framework/config"
	"github.com/goravel/framework/support"

	"github.com/vortechron/reverb-go/broadcast"
)

// newConfig returns a real Goravel config that reads the process environment.
func newConfig(t *testing.T, env map[string]string) *goravelconfig.Application {
	t.Helper()
	support.DontVerifyAppKey = true
	for k, v := range env {
		t.Setenv(k, v)
	}
	cfg := goravelconfig.NewApplication("")
	cfg.Add("broadcasting", Config(cfg.Env))

	return cfg
}

func TestSettingsReadLaravelEnvNames(t *testing.T) {
	cfg := newConfig(t, map[string]string{
		"BROADCAST_CONNECTION": "reverb", "REVERB_APP_ID": "app", "REVERB_APP_KEY": "key", "REVERB_APP_SECRET": "secret",
		"REVERB_HOST": "realtime.example.com", "REVERB_PORT": "6101", "REVERB_SCHEME": "http",
	})

	s, err := SettingsFrom(cfg)

	require.NoError(t, err)
	assert.Equal(t, DriverReverb, s.Driver)
	assert.Equal(t, broadcast.Credentials{AppID: "app", Key: "key", Secret: "secret"}, s.Credentials)
	assert.Equal(t, "http://realtime.example.com:6101", s.BaseURL)
}

func TestSettingsDefaultToNull(t *testing.T) {
	s, err := SettingsFrom(newConfig(t, map[string]string{"BROADCAST_CONNECTION": ""}))

	require.NoError(t, err)
	assert.Equal(t, DriverNull, s.Driver)
}

func TestSettingsReverbNeedsCredentialsAndHost(t *testing.T) {
	_, err := SettingsFrom(newConfig(t, map[string]string{"BROADCAST_CONNECTION": "reverb"}))
	assert.Error(t, err)
}

func TestSettingsUnknownDriver(t *testing.T) {
	_, err := SettingsFrom(newConfig(t, map[string]string{"BROADCAST_CONNECTION": "pusher"}))
	assert.Error(t, err)
}

type fakeInfo struct{ lines []string }

func (f *fakeInfo) Infof(format string, _ ...any) { f.lines = append(f.lines, format) }

func TestNewPublisherPerDriver(t *testing.T) {
	log := &fakeInfo{}

	null, err := NewPublisher(Settings{Driver: DriverNull}, log)
	require.NoError(t, err)
	require.NoError(t, null.Trigger(context.Background(), []string{"a"}, "E", nil))
	assert.Empty(t, log.lines)

	logging, err := NewPublisher(Settings{Driver: DriverLog}, log)
	require.NoError(t, err)
	require.NoError(t, logging.Trigger(context.Background(), []string{"a"}, "E", nil))
	assert.Len(t, log.lines, 1)

	api := newCapture(t)
	live, err := NewPublisher(Settings{Driver: DriverReverb, Credentials: testCreds, BaseURL: api.srv.URL}, log)
	require.NoError(t, err)
	require.NoError(t, live.Trigger(context.Background(), []string{"a"}, "E", map[string]any{"x": 1}))
	assert.Len(t, api.bodies, 1)

	_, err = NewPublisher(Settings{Driver: "other"}, log)
	assert.Error(t, err)
}
