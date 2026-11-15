# Differences from Laravel

reverb-go passed the same 66 conformance checks as Laravel Reverb v1.11.1. These differences come from reading Reverb's source and are deliberate.

## The server

| Reverb | reverb-go | Why |
|---|---|---|
| A whisper on a cache channel updates the cached event | It does not | A subscriber could overwrite what later subscribers receive. |
| A whisper on a public channel is allowed when the sender is subscribed | Refused with 4009 | Pusher allows client events on private and presence channels only. |
| HTTP API `auth_timestamp` is not checked | Must be within 600 seconds, else 401 | Blocks replays. Pusher's rule. |
| HTTP API `auth_key` is not compared | Must equal the app key, else 401 | Pusher's rule. |
| An origin or quota refusal sends `pusher:error` and leaves the socket open | Sends `pusher:error`, then closes with the same code | No half-open sockets. |
| An empty `channels` list encodes as `[]` | `{}` | Clients expect an object. |
| A presence subscribe without `channel_data` is accepted if signed, then every member list is sent empty | Refused with 4009 | One client could blank the member list for everyone. |
| Rate limiting is off by default, 60 messages per 60 s | On by default, 600 per 60 s, error 4301 | Abuse protection by default. |
| No limit on channels per connection, presence members or `channel_data` size | 100 channels (4100), 100 members (4100), 5000 bytes (4200), all configurable | Abuse protection. |
| No limit on batch size, channels per event or event name length | 10 events, 100 channels, 200 characters, answered with 422 | Pusher's limits. Laravel never sends more. |
| No deadline on API requests | The body must arrive within 10 s (408) | Blocks slow-body attacks. WebSockets are not affected. |
| A presence list keeps join order | Not ordered | Join and leave stay O(1). pusher-js does not depend on the order. |
| Redis messages are PHP-serialized and unsigned | JSON signed with HMAC (`REVERB_SCALING_SECRET`); unsigned messages are dropped | Anyone who can write to Redis could otherwise inject events. A reverb-go node cannot join a PHP Reverb cluster. |

## Broadcasting from Go

| Laravel | reverb-go | Why |
|---|---|---|
| Broadcast event classes (`ShouldBroadcast`) | `Broadcast(ctx, channels, name, payload)` | Go has no event classes. You pass what the class would send. |
| Channel names without a prefix in `broadcastOn()` (`new PrivateChannel('orders.5')`) | Wire names with the prefix (`private-orders.5`) | There are no channel objects. |

## Not built yet

| Laravel | Status |
|---|---|
| TLS in the process (`REVERB_TLS_*`) | Not built. Use a reverse proxy. |
| Monitoring with Laravel Pulse | Not built. |
| Reverb's events (`MessageReceived`, `MessageSent`, `ChannelCreated`, `ChannelRemoved`, `ConnectionPruned`) | Not built. |
| Presence lists, channel info and `/connections` merged across nodes | Not built. They are per node. |
| Queued broadcasting (`ShouldBroadcast` on a queue), `ShouldBroadcastNow`, `broadcastQueue()`, `broadcastWhen()`, `ShouldDispatchAfterCommit` | Not ported. Broadcast from your own queue job, after commit. |
| Anonymous events (`Broadcast::on(...)->send()`) | Not ported. `Broadcast` already takes a name and a payload. |
| Rescuing broadcasts (`ShouldRescue`) | Not ported. Handle the returned error. |
| Model broadcasting (`BroadcastsEvents`) | Not ported. |
| Channel classes | Not ported. A rule is a function. |
| Broadcast notifications | Not ported in notifications-go yet. |
| Pusher Channels and Ably as Go drivers | Not built. Pusher uses the same HTTP API, but only reverb-go and Reverb are tested. |

## Laravel-only features

| Laravel | Why it is not in Go |
|---|---|
| `php artisan install:broadcasting`, `reverb:install` | Set the env and add the provider. |
| The `Broadcast` facade | Use `reverbgo.BroadcasterFrom(app)` and `reverbgo.RegistryFrom(app)`. |
| Echo's React and Vue hooks | Browser code: unchanged, they work with reverb-go. |
