package goravel

import (
	"fmt"

	contractsfoundation "github.com/goravel/framework/contracts/foundation"

	"github.com/vortechron/reverb-go/broadcast"
)

// Container keys. The provider binds them; there are no package level values.
const (
	BindingPublisher   = "reverb.publisher"
	BindingBroadcaster = "reverb.broadcaster"
	BindingRegistry    = "reverb.registry"
	BindingCredentials = "reverb.credentials" //nolint:gosec // a container key, not a secret
)

// ServiceProvider binds the broadcasting services from the "broadcasting"
// config (see Config). List it after the framework providers, then register
// channel rules on RegistryFrom(app) in your routes.
//
// Register panics on a bad config, such as BROADCAST_CONNECTION=reverb with no
// REVERB_APP_KEY, so the app never starts half configured.
type ServiceProvider struct{}

// Register fills in the default config when the app has none, then binds.
func (ServiceProvider) Register(app contractsfoundation.Application) {
	cfg := app.MakeConfig()
	if cfg.GetString("broadcasting.default") == "" {
		cfg.Add("broadcasting", Config(cfg.Env))
	}
	settings, err := SettingsFrom(cfg)
	if err != nil {
		panic(err)
	}

	app.Singleton(BindingCredentials, func(contractsfoundation.Application) (any, error) {
		return settings.Credentials, nil
	})
	app.Singleton(BindingRegistry, func(contractsfoundation.Application) (any, error) {
		return NewRegistry(), nil
	})
	app.Singleton(BindingPublisher, func(a contractsfoundation.Application) (any, error) {
		pub, err := NewPublisher(settings, a.MakeLog())
		if err != nil {
			return nil, fmt.Errorf("bind publisher: %w", err)
		}

		return pub, nil
	})
	app.Singleton(BindingBroadcaster, func(a contractsfoundation.Application) (any, error) {
		return NewBroadcaster(PublisherFrom(a)), nil
	})
}

// Boot has nothing to do. Rules are added by the app, in routes/channels.go.
func (ServiceProvider) Boot(contractsfoundation.Application) {}

func resolve[T any](app contractsfoundation.Application, key string) (T, bool) {
	var zero T
	instance, err := app.Make(key)
	if err != nil {
		return zero, false
	}
	v, ok := instance.(T)

	return v, ok
}

// PublisherFrom returns the bound publisher, or nil.
func PublisherFrom(app contractsfoundation.Application) Publisher {
	p, _ := resolve[Publisher](app, BindingPublisher)

	return p
}

// BroadcasterFrom returns the bound broadcaster, or nil.
func BroadcasterFrom(app contractsfoundation.Application) *Broadcaster {
	b, _ := resolve[*Broadcaster](app, BindingBroadcaster)

	return b
}

// RegistryFrom returns the channel registry, or nil.
func RegistryFrom(app contractsfoundation.Application) *Registry {
	r, _ := resolve[*Registry](app, BindingRegistry)

	return r
}

// CredentialsFrom returns the app credentials.
func CredentialsFrom(app contractsfoundation.Application) (broadcast.Credentials, bool) {
	return resolve[broadcast.Credentials](app, BindingCredentials)
}

// AuthHandlerFrom builds the /broadcasting/auth handler from the bound
// credentials and registry.
func AuthHandlerFrom(app contractsfoundation.Application, user UserResolver, log Logger) (*AuthHandler, error) {
	creds, _ := CredentialsFrom(app)

	return NewAuthHandler(AuthConfig{Credentials: creds, Registry: RegistryFrom(app), User: user, Log: log})
}
