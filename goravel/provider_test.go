package goravel

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	contractsfoundation "github.com/goravel/framework/contracts/foundation"
	mocksfoundation "github.com/goravel/framework/mocks/foundation"
	mockslog "github.com/goravel/framework/mocks/log"
)

// containerApp is an app mock whose Singleton and Make work like a container.
func containerApp(t *testing.T, env map[string]string) *mocksfoundation.Application {
	t.Helper()
	cfg := newConfig(t, env)
	factories := map[string]func(contractsfoundation.Application) (any, error){}
	built := map[string]any{}

	app := mocksfoundation.NewApplication(t)
	app.EXPECT().MakeConfig().Return(cfg).Maybe()
	app.EXPECT().MakeLog().Return(mockslog.NewLog(t)).Maybe()
	app.EXPECT().Singleton(mock.Anything, mock.Anything).Run(func(key any, f func(contractsfoundation.Application) (any, error)) {
		factories[key.(string)] = f
	}).Maybe()
	app.EXPECT().Make(mock.Anything).RunAndReturn(func(key any) (any, error) {
		k := key.(string)
		if v, ok := built[k]; ok {
			return v, nil
		}
		factory, ok := factories[k]
		if !ok {
			return nil, errors.New("not bound")
		}
		v, err := factory(app)
		if err == nil {
			built[k] = v
		}

		return v, err
	}).Maybe()

	return app
}

func TestProviderBindsPublisherBroadcasterRegistryAndCredentials(t *testing.T) {
	app := containerApp(t, map[string]string{
		"BROADCAST_CONNECTION": "reverb", "REVERB_APP_ID": "app", "REVERB_APP_KEY": "key", "REVERB_APP_SECRET": "secret",
		"REVERB_HOST": "127.0.0.1", "REVERB_PORT": "1", "REVERB_SCHEME": "http",
	})

	(&BroadcastServiceProvider{}).Register(app)

	reg1, reg2 := RegistryFrom(app), RegistryFrom(app)
	require.NotNil(t, reg1)
	assert.Same(t, reg1, reg2, "one registry per app")
	assert.NotNil(t, BroadcasterFrom(app))
	assert.NotNil(t, PublisherFrom(app))
	creds, ok := CredentialsFrom(app)
	require.True(t, ok)
	assert.Equal(t, "key", creds.Key)
}

func TestProviderBadConfigPanicsAtRegister(t *testing.T) {
	app := containerApp(t, map[string]string{"BROADCAST_CONNECTION": "reverb"})

	assert.Panics(t, func() { (&BroadcastServiceProvider{}).Register(app) })
}

func TestFromHelpersReturnNilWhenNothingIsBound(t *testing.T) {
	app := containerApp(t, nil)

	assert.Nil(t, RegistryFrom(app))
	assert.Nil(t, PublisherFrom(app))
	assert.Nil(t, BroadcasterFrom(app))
	_, ok := CredentialsFrom(app)
	assert.False(t, ok)
}

func TestAuthHandlerFromApp(t *testing.T) {
	app := containerApp(t, map[string]string{
		"BROADCAST_CONNECTION": "log", "REVERB_APP_ID": "app", "REVERB_APP_KEY": "key", "REVERB_APP_SECRET": "secret",
	})
	(&BroadcastServiceProvider{}).Register(app)

	h, err := AuthHandlerFrom(app, signedIn("1"), nil)

	require.NoError(t, err)
	assert.NotNil(t, h)
}

func TestProviderBootDoesNothingYet(t *testing.T) {
	(&BroadcastServiceProvider{}).Boot(containerApp(t, nil))
}
