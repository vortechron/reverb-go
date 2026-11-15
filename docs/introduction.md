# Introduction

reverb-go is a Pusher Channels (protocol 7) WebSocket server written in Go. It is a drop-in for [Laravel Reverb](https://github.com/laravel/reverb): Laravel Echo and pusher-js connect with no client change, and a Laravel app publishes to it through the same Pusher HTTP API.

It also ships two Go packages:

| Package | Holds |
|---|---|
| `github.com/vortechron/reverb-go/broadcast` | Publish events over the Pusher HTTP API and sign `/broadcasting/auth` answers. No framework import. It also works against Laravel Reverb. |
| `github.com/vortechron/reverb-go/goravel` | Goravel glue: the broadcasting config, a service provider, channel rules like `routes/channels.php`, the auth route and a broadcaster with `toOthers()`. |

The examples import the adapter as `reverbgo`.

reverb-go is an independent project. It is not made by, affiliated with or endorsed by Laravel or Pusher. "Laravel", "Laravel Reverb" and "Pusher" are trademarks of their owners.

## What it covers

| Area | Status |
|---|---|
| Public, private and presence channels, with HMAC auth | Ported |
| Cache channels (`cache-`, `private-cache-`, `presence-cache-`) | Ported |
| Client events (whispers) | Ported |
| The Pusher HTTP API: events, batch events, channels, users, connections, terminate | Ported |
| Allowed origins, multiple apps, connection and message limits | Ported |
| Scaling across nodes with Redis | Ported, with limits (see [Scaling](production#scaling)) |
| Laravel's broadcasting on the Go side: channel rules, `/broadcasting/auth`, `broadcast()` with `toOthers()` | Ported in the `goravel` package |
| TLS in the process, Pulse monitoring, Reverb's events | Not built |

## Conformance

One suite of 66 checks ran against Laravel Reverb v1.11.1 and against reverb-go, with pusher-js 8.6.0, Laravel Echo 1.19.0 and 2.6.3, and the real pusher-php-server 7.3.0 publisher. Both servers passed all 66. The deliberate differences are in [Differences from Laravel](differences).

## Status

The current release is v0.2.x. reverb-go is public and MIT licensed. Until 1.0, a minor release can still rename something, so pin the version in `go.mod`.
