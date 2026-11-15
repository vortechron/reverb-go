# Configuration

The server reads its settings from environment variables. The names match Laravel Reverb's.

## Application credentials

Every connection and API call belongs to an app. Set one app with three variables:

```ini
REVERB_APP_ID=my-app-id
REVERB_APP_KEY=my-app-key
REVERB_APP_SECRET=my-app-secret
```

The key goes to the browser. The secret signs API calls and channel auth, so keep it on the server.

## Allowed origins

`REVERB_ALLOWED_ORIGINS` limits which sites may open a WebSocket. It is a comma list of hosts, and `*` works inside a pattern:

```ini
REVERB_ALLOWED_ORIGINS=example.com,*.example.com
```

The default is `*`, every origin. A request without an `Origin` header is refused when origins are restricted. A refused connection gets a `pusher:error` and the socket closes.

## Additional applications

One server can serve several apps. Put them in a JSON file and point `REVERB_APPS_FILE` at it. The file replaces the three app variables:

```json
{"apps":[{"id":"1","key":"k1","secret":"s1","allowed_origins":["app.example.com"],
          "activity_timeout":30,"ping_interval":60,"max_message_size":10000,
          "max_connections":0,"accept_client_events_from":"members"}]}
```

An app entry also takes `max_channels_per_connection`, `max_presence_members`, `max_channel_data_size` and a `rate_limiting` object with `enabled`, `max_attempts`, `decay_seconds` and `terminate_on_limit`.

## Server

| Variable | Default | Meaning |
|---|---|---|
| `REVERB_SERVER_HOST` / `REVERB_SERVER_PORT` | `0.0.0.0` / `8080` | The listen address. |
| `REVERB_SERVER_PATH` | empty | A URL prefix. It is removed before API signatures are checked, like Reverb. |
| `REVERB_MAX_REQUEST_SIZE` | `10000` | The largest HTTP API body, in bytes. |

## Per-app limits

| Variable | Default | Meaning |
|---|---|---|
| `REVERB_APP_PING_INTERVAL` | `60` | Seconds. Silent clients are pinged, then dropped (error 4201). |
| `REVERB_APP_ACTIVITY_TIMEOUT` | `30` | Seconds, sent to clients in `connection_established`. |
| `REVERB_APP_MAX_CONNECTIONS` | none | Over the limit: error 4004. |
| `REVERB_APP_MAX_MESSAGE_SIZE` | `10000` | Bytes. A bigger client message closes the connection. |
| `REVERB_APP_ACCEPT_CLIENT_EVENTS_FROM` | `members` | `members`, `all`, or `none` (client events off). |
| `REVERB_APP_MAX_CHANNELS_PER_CONNECTION` | `100` | Over the limit: error 4100. |
| `REVERB_APP_MAX_PRESENCE_MEMBERS` | `100` | Distinct users per presence channel. Over the limit: error 4100. |
| `REVERB_APP_MAX_CHANNEL_DATA_SIZE` | `5000` | Bytes of `channel_data` (user id and info). Over the limit: error 4200. |
| `REVERB_APP_RATE_LIMITING_ENABLED` | `true` | Reverb's option, on by default here. |
| `REVERB_APP_RATE_LIMIT_MAX_ATTEMPTS` / `REVERB_APP_RATE_LIMIT_DECAY_SECONDS` | `600` / `60` | Client messages per window. Over: error 4301. |
| `REVERB_APP_RATE_LIMIT_TERMINATE` | `false` | Also disconnect when over the limit. |

## Scaling

| Variable | Default | Meaning |
|---|---|---|
| `REVERB_SCALING_ENABLED` | `false` | Share events between nodes through Redis. |
| `REVERB_SCALING_SECRET` | required when scaling is on | Signs every Redis message (HMAC-SHA256). Use a long random value, the same on all nodes. |
| `REVERB_SCALING_CHANNEL` | `reverb-go` | The Redis pub/sub channel. |
| `REDIS_HOST` / `REDIS_PORT` / `REDIS_PASSWORD` / `REDIS_DB` | `127.0.0.1` / `6379` / none / `0` | Redis. |

See [Scaling](production#scaling).

## SSL

reverb-go does not terminate TLS. Put Caddy, nginx or a Kubernetes ingress in front, and let clients connect with `wss` to the proxy. See [Web server](production#web-server).
