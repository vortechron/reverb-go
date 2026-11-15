# Protocol Coverage

reverb-go speaks Pusher protocol 7, as Laravel Reverb does.

| Feature | Status |
|---|---|
| `GET /app/{key}` upgrade, optional path prefix | Yes |
| `pusher:connection_established` (socket id `N.N`, `activity_timeout`) | Yes |
| `pusher:ping` and `pusher:pong`, server pings, 4201 on no reply | Yes |
| WebSocket ping and pong frames | Answered by the library, count as activity |
| Public, `private-` and `presence-` subscribe, `pusher:unsubscribe` | Yes |
| HMAC check of `auth` (and `channel_data` for presence) | Yes |
| Presence: `subscription_succeeded` with count, ids and hash, `member_added`, `member_removed` (once per user, not per socket) | Yes |
| `client-*` events: members only, not echoed to the sender, `user_id` added on presence | Yes |
| `cache-`, `private-cache-`, `presence-cache-` channels (`cache_miss`, replay) | Yes |
| Errors 4001, 4004, 4009, 4100, 4200, 4201, 4301 | Yes |
| Max message size, allowed origins, per-app client events on or off | Yes |
| `POST /apps/{id}/events`, `/batch_events` (`socket_id` exclusion, `info`) | Yes |
| `GET /apps/{id}/channels`, `/channels/{name}`, `/channels/{name}/users`, `/connections` | Yes |
| `POST /apps/{id}/users/{user}/terminate_connections` | Yes |
| Request signing: `auth_key`, `auth_timestamp` (600 s), `auth_version`, `body_md5`, `auth_signature` | Yes |
| `pusher:signin`, watchlist, webhooks | No (Reverb has none either) |

## Error codes

| Code | Meaning |
|---|---|
| 4001 | Unknown app key. |
| 4004 | Over the app's connection limit. |
| 4009 | Unauthorized: a bad auth signature, a presence subscribe without a user id, or a client event on a channel the socket is not on. |
| 4100 | Over the channels per connection or presence members limit. |
| 4200 | Bad input, or `channel_data` over its size limit. |
| 4201 | No pong in time. |
| 4301 | Over the client message rate limit. |

## HTTP API limits

A batch holds at most 10 events, an event goes to at most 100 channels, and an event name is at most 200 characters. Over those limits, the answer is 422. `auth_timestamp` must be within 600 seconds and `auth_key` must equal the app key, else 401.
