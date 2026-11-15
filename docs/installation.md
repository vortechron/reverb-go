# Installation

## Requirements

- Go 1.26 or later to build the server or use the packages.
- Goravel v1.18 for the `goravel` package.

reverb-go is a public module. No access setup is needed.

## Installing the server

Build the binary from the module:

```shell
go install github.com/vortechron/reverb-go/cmd/reverb-go@latest
```

Or from a clone:

```shell
go build -o reverb-go ./cmd/reverb-go
```

Start it with the app credentials:

```shell
REVERB_APP_ID=app REVERB_APP_KEY=key REVERB_APP_SECRET=secret ./reverb-go
# listens on 0.0.0.0:8080 ; health check: GET /up
```

There is no Dockerfile. TLS is not done in the process: put a reverse proxy in front and use `wss`. See [Running in production](production).

## Pointing a Laravel app at it

Laravel already talks to Reverb with the Pusher HTTP API, so only the `.env` values change:

```ini
BROADCAST_CONNECTION=reverb
REVERB_APP_ID=app
REVERB_APP_KEY=key
REVERB_APP_SECRET=secret
REVERB_HOST=realtime.example.com
REVERB_PORT=443
REVERB_SCHEME=https
```

`REVERB_HOST` is where reverb-go is reachable. The same values go in the `VITE_REVERB_*` variables that Echo reads. Stop `php artisan reverb:start`.

## Installing the Go packages

```shell
go get github.com/vortechron/reverb-go
```

### Goravel

Add the broadcasting config in `config/broadcasting.go`. `reverbgo.Config` returns the shape of Laravel's `config/broadcasting.php`, read from the same env names:

```go
config := facades.Config()
config.Add("broadcasting", reverbgo.Config(config.Env))
```

Then register the provider in `bootstrap/providers.go`, after the framework providers:

```go
func Providers() []foundation.ServiceProvider {
	return []foundation.ServiceProvider{
		// ... the framework providers first ...
		&reverbgo.BroadcastServiceProvider{},
	}
}
```

| Env | Meaning |
|---|---|
| `BROADCAST_CONNECTION` | `reverb` (the Pusher HTTP API), `log` or `null`. Default `null`. |
| `REVERB_APP_ID`, `REVERB_APP_KEY`, `REVERB_APP_SECRET` | The app credentials. |
| `REVERB_HOST`, `REVERB_PORT`, `REVERB_SCHEME` | Where the server's HTTP API listens. Defaults `443` and `https`. |

The provider panics at boot on a bad config, such as `BROADCAST_CONNECTION=reverb` with no `REVERB_APP_KEY`, so the app never starts half configured. It binds the publisher, the broadcaster, the channel registry and the credentials under `reverb.publisher`, `reverb.broadcaster`, `reverb.registry` and `reverb.credentials`.

> [!NOTE]
> The type is `BroadcastServiceProvider`, not `ServiceProvider`. Goravel keys providers by their type name, so two packages that both export `ServiceProvider` from a package called `goravel` would collide.

Next, add the [channel rules and the auth route](authorizing-channels).
