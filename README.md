# reverb-go

A Pusher Channels (protocol 7) WebSocket server in Go. It is a drop-in for
[Laravel Reverb](https://github.com/laravel/reverb): Laravel Echo and pusher-js connect with
no client change. It also ships a Go package, `broadcast`, for publishing events and for
answering `/broadcasting/auth`.

## Run it

```bash
go build -o reverb-go ./cmd/reverb-go
REVERB_APP_ID=app REVERB_APP_KEY=key REVERB_APP_SECRET=secret ./reverb-go
# listens on 0.0.0.0:8080 ; health check: GET /up
```

There is no Dockerfile. TLS is not done in the process: put a reverse proxy (Caddy, nginx,
a Kubernetes ingress) in front and use `wss`.

### Environment

Names match Laravel Reverb.

| Variable | Default | Meaning |
|---|---|---|
| `REVERB_APP_ID` / `REVERB_APP_KEY` / `REVERB_APP_SECRET` | required | The app. Or use `REVERB_APPS_FILE` |
| `REVERB_APPS_FILE` | none | JSON file with many apps (below). Replaces the three variables above |
| `REVERB_SERVER_HOST` / `REVERB_SERVER_PORT` | `0.0.0.0` / `8080` | Listen address |
| `REVERB_SERVER_PATH` | empty | URL prefix. It is removed before API signatures are checked, like Reverb |
| `REVERB_MAX_REQUEST_SIZE` | `10000` | Max HTTP API body, bytes |
| `REVERB_ALLOWED_ORIGINS` | `*` | Comma list. Host only, `*` works inside a pattern (`*.example.com`) |
| `REVERB_APP_PING_INTERVAL` | `60` | Seconds. Silent clients are pinged, then dropped (error 4201) |
| `REVERB_APP_ACTIVITY_TIMEOUT` | `30` | Seconds, sent to clients in `connection_established` |
| `REVERB_APP_MAX_CONNECTIONS` | none | Over the limit: error 4004 |
| `REVERB_APP_MAX_MESSAGE_SIZE` | `10000` | Bytes. A bigger client message closes the connection |
| `REVERB_APP_ACCEPT_CLIENT_EVENTS_FROM` | `members` | `members`, `all`, or `none` (off) |
| `REVERB_APP_MAX_CHANNELS_PER_CONNECTION` | `100` | Over the limit: error 4100 |
| `REVERB_APP_MAX_PRESENCE_MEMBERS` | `100` | Distinct users per presence channel. Over the limit: error 4100 |
| `REVERB_APP_MAX_CHANNEL_DATA_SIZE` | `5000` | Bytes of `channel_data` (user id and info). Over the limit: error 4200 |
| `REVERB_APP_RATE_LIMITING_ENABLED` | `true` | Reverb's option, but on by default here |
| `REVERB_APP_RATE_LIMIT_MAX_ATTEMPTS` / `_DECAY_SECONDS` | `600` / `60` | Client messages per window. Over: error 4301 |
| `REVERB_APP_RATE_LIMIT_TERMINATE` | `false` | Also disconnect when over the limit |
| `REVERB_SCALING_ENABLED` | `false` | Share events between nodes through Redis |
| `REVERB_SCALING_SECRET` | required when scaling is on | Signs every Redis message (HMAC-SHA256). Use a long random value, the same on all nodes |
| `REVERB_SCALING_CHANNEL` | `reverb-go` | Redis pub/sub channel |
| `REDIS_HOST` / `REDIS_PORT` / `REDIS_PASSWORD` / `REDIS_DB` | `127.0.0.1` / `6379` / none / `0` | Redis |

Multi-app file:

```json
{"apps":[{"id":"1","key":"k1","secret":"s1","allowed_origins":["app.example.com"],
          "activity_timeout":30,"ping_interval":60,"max_message_size":10000,
          "max_connections":0,"accept_client_events_from":"members"}]}
```

### Point a Laravel app at it

Laravel already talks to Reverb with the Pusher HTTP API, so only the `.env` values change:

```dotenv
BROADCAST_CONNECTION=reverb
REVERB_APP_ID=app
REVERB_APP_KEY=key
REVERB_APP_SECRET=secret
REVERB_HOST=realtime.example.com   # where reverb-go is reachable
REVERB_PORT=443
REVERB_SCHEME=https
```

The same values go in the `VITE_REVERB_*` variables that Echo reads. Stop `php artisan reverb:start`.

## Protocol coverage

| Feature | Status |
|---|---|
| `GET /app/{key}` upgrade (`protocol=7` ignored, like Reverb), optional path prefix | yes |
| `pusher:connection_established` (socket id `N.N`, `activity_timeout`) | yes |
| `pusher:ping` / `pusher:pong`, server pings, 4201 on no reply | yes |
| WebSocket ping/pong frames | answered by the library, count as activity |
| Public, `private-`, `presence-` subscribe and `pusher:unsubscribe` | yes |
| HMAC check of `auth` (and `channel_data` for presence) | yes |
| Presence: `subscription_succeeded` with count/ids/hash, `member_added`, `member_removed` (once per user, not per socket) | yes |
| `client-*` events (whisper): members only, not echoed to sender, `user_id` added on presence | yes |
| `cache-`, `private-cache-`, `presence-cache-` channels (`cache_miss`, replay) | yes |
| Errors 4001, 4004, 4009, 4200, 4201, 4301 | yes |
| Max message size, allowed origins, per-app client events on/off | yes |
| Slow clients: bounded queue (256 frames), disconnected when full | yes |
| Abuse limits: channels per connection, presence members, `channel_data` size, message rate limit | yes (defaults in the env table) |
| Presence subscribe without a user id | refused (4009) |
| `POST /apps/{id}/events`, `/batch_events` (`socket_id` exclusion, `info`) | yes |
| `GET /apps/{id}/channels`, `/channels/{name}`, `/channels/{name}/users`, `/connections` | yes |
| `POST /apps/{id}/users/{user}/terminate_connections` | yes |
| Request signing: `auth_key`, `auth_timestamp` (600 s), `auth_version`, `body_md5`, `auth_signature` | yes |
| `GET /up` | yes (503 while the Redis bus is down) |
| `pusher:signin`, watchlist, webhooks | no (Reverb has none either) |

Conformance results for Laravel Reverb and reverb-go: [conformance/RESULTS.md](conformance/RESULTS.md).

## WebSocket library

[`coder/websocket`](https://github.com/coder/websocket). It takes a `context` on every read and
write, so timeouts and shutdown are simple. It has a read limit (`SetReadLimit`) that maps
straight to `max_message_size`, no dependencies, and it is actively maintained. gorilla/websocket
would also work, but it needs manual deadlines and does not support concurrent writes, which we
avoid with one writer goroutine per connection anyway.

## Design

- One read goroutine (the HTTP handler) and one write goroutine per connection.
- Each connection has a bounded send queue. `Send` never blocks. A full queue closes the socket.
- Channel state lives in `internal/hub` behind one mutex per app. Delivery only enqueues frames.
- Connection limit check and insert are one atomic step (`Hub.TryAdd`).
- With Redis, every node still delivers locally first and queues a signed message for Redis. A
  bounded queue and one sender goroutine mean a slow Redis never blocks a client or an API call
  (a full queue drops the message and logs it). Messages carry the node id, so a node ignores its own.
- Fan-out to clients runs on a snapshot of the subscribers, outside the hub lock.
- HTTP API requests have a read deadline (10 s, set per request, so WebSockets are not cut).

## Go package `broadcast`

```go
import "github.com/vortechron/reverb-go/broadcast"

creds := broadcast.Credentials{AppID: "app", Key: "key", Secret: "secret"}

// Publish (also works against Laravel Reverb)
pub := broadcast.NewPublisher(broadcast.Config{Credentials: creds, BaseURL: "http://127.0.0.1:8080"})
err := pub.Trigger(ctx, []string{"private-users.5"}, `App\Events\CallEnded`,
    map[string]any{"id": 9}, broadcast.WithSocketID(broadcast.SocketIDFromRequest(r)))
err = pub.TriggerBatch(ctx, []broadcast.Event{{Channel: "news", Name: "Post", Data: map[string]any{"id": 1}}})

// /broadcasting/auth: accepts a form body or a JSON body
func authHandler(w http.ResponseWriter, r *http.Request) {
    req, err := broadcast.ParseAuthRequest(r)            // validates socket_id and channel_name
    if err != nil { http.Error(w, err.Error(), http.StatusUnprocessableEntity); return }
    // ... check the user may join req.ChannelName ...
    resp, err := creds.AuthorizePrivate(req.SocketID, req.ChannelName)
    // presence: creds.AuthorizePresence(req.SocketID, req.ChannelName,
    //     broadcast.PresenceMember{UserID: user.ID, UserInfo: map[string]any{"name": user.Name}})
    json.NewEncoder(w).Encode(resp)
}
```

| Function | Purpose |
|---|---|
| `NewPublisher(Config)`, `(*Publisher).Trigger`, `TriggerBatch` | Signed publish. More than 100 channels are split into several requests. A string `data` is sent as is, anything else as JSON |
| `WithSocketID(id)`, `SocketIDFromRequest(r)` | Exclude the sender (Echo sends `X-Socket-Id`) |
| `Credentials.AuthorizePrivate`, `AuthorizePresence` | Return `(AuthResponse, error)`. They check `socket_id` (`^\d+\.\d+$`) and channel name (`^[-a-zA-Z0-9_=@,.;]{1,200}$`) before signing |
| `ParseAuthRequest(r)` | Reads `socket_id` and `channel_name` from a form or JSON body, with the same checks |
| `RequestSignature`, `HMACHex`, `ValidSocketID`, `ValidChannelName` | Building blocks |

### Event names and payloads

A Go app must send exactly what Laravel would, or Echo listeners will not match:

- A Laravel event class is sent under its full class name, for example `App\Events\CallEnded`.
  Echo's `.listen('CallEnded')` matches that name. If the event defines `broadcastAs()`, use
  that string, and listen with a leading dot: `.listen('.my-name')`.
- The payload is the event's public properties at the top level, or the array that
  `broadcastWith()` returns. Laravel adds nothing else.
- Channel names keep their type prefix on the wire: `private-calls.5`, `presence-room.1`.
  Echo adds the prefix for you; Go code must write it.

## Scaling with Redis, and its limits

With `REVERB_SCALING_ENABLED=true` each node publishes events (including whispers) and
terminate requests to a Redis channel, and every other node delivers them to its own clients.

- Redis must have a password or ACL, and should not be reachable from the internet. Every message is
  signed with `REVERB_SCALING_SECRET` and nodes drop unsigned or badly signed ones, but a password is
  still the first line of defence. If Redis is down at start the node retries with backoff, and `/up`
  answers 503 until the subscription is back.
- The message format is reverb-go's own JSON. **A reverb-go node cannot join a PHP Reverb
  cluster** (Reverb publishes PHP-serialized objects). Replace all nodes at once.
- Presence lists, channel info and connection counts are per node. In a cluster, a presence
  channel only shows users connected to the same node. Use one node, or sticky routing per
  channel, if you rely on presence. This is the biggest known gap.

## Known gaps

- No in-process TLS, no Pulse or Telescope hooks, no webhooks.
- Presence, channel info and `/connections` are not merged across nodes.
- Differences from Reverb are listed in [conformance/RESULTS.md](conformance/RESULTS.md).

## Develop

```bash
go test -race ./...            # unit and integration tests
golangci-lint run ./...
sh scripts/coverage-gate.sh 80 coverage.out
./conformance/run-reverb-go.sh # needs Node 20+; npm install first
./conformance/run-reverb.sh    # same suite against Laravel Reverb (needs the werewolf app)
```

Browser demo: `examples/echo-page` (see its `server.mjs`).
