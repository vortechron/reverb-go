# Conformance results

Date: 2026-11-05. The same suite (`conformance/suite/*.test.mjs`, 61 checks) ran
against both servers on this Mac. Both passed all 61.

| Target | How it was started | Result |
|---|---|---|
| Laravel Reverb v1.11.1 (PHP 8.4.25) | `run-reverb.sh`: `php artisan reverb:start --host=127.0.0.1 --port=6001` from `/Users/amiruladli/Projects/werewolf`, env overrides only, `APP_CONFIG_CACHE=/tmp/reverb-conf-missing-config.php`, no file edited | 61 passed, 0 failed |
| reverb-go (this repo) | `run-reverb-go.sh`: built binary on `127.0.0.1:6002` | 61 passed, 0 failed (run twice, same result) |

Clients used (Node v22.14.0): pusher-js 8.6.0, laravel-echo 1.19.0 (alias `echo1`),
laravel-echo 2.6.3 (alias `echo2`), ws 8.22.1.

## What is checked

For each of the three client stacks (plain pusher-js, Echo 1.x, Echo 2.x):

- connect, socket id format, error 4001 for a wrong app key, error 4009 for a forged auth signature
- public channel event from the HTTP API (event name with backslashes: `App\Events\CallEnded`)
- private channel through an auth endpoint (form body) and through a JSON-body authorizer
  (same shape as `terapeas-portal/resources/js/websocket.ts`)
- `socket_id` exclusion, one event to several channels, unsubscribe
- whisper between two private-channel clients, not echoed to the sender
- presence: members on join, `member_added`, `member_removed`, one user with two sockets counts once
- cache channel replays the last server event to a late subscriber
- Echo only: `listen('CallEnded')` matches `App\Events\CallEnded`

Raw frame checks (one socket, no client library): `connection_established` shape,
`pusher:ping` to `pusher:pong`, empty `subscription_succeeded` data, bad or foreign
signatures (4009), tampered presence `channel_data` (4009), garbage input (4200),
client event from a socket that is not subscribed (4009).

HTTP API checks: `/up`, signed event, wrong signature (401), changed query (401),
unknown app (404), missing data (422), batch events, channels list and filter, one
channel, connection count, presence users (200, 400, 404), terminate connections,
`info` flag on events.

The suite signs its own API calls in Node (`lib/signing.mjs`), so it does not share
code with the Go signer.

## Where Reverb and reverb-go differ (not covered by the suite)

These come from reading Reverb v1.11.1 source, not from a live test.

| Topic | Reverb | reverb-go | Why |
|---|---|---|---|
| Whisper on a cache channel | Updates the cached event (`CacheChannel::broadcast` stores every payload, and client events use it) | Does **not** update the cache | A subscriber could overwrite what later subscribers receive. No conformance check needs the Reverb behaviour. |
| Whisper on a public channel | Allowed when the sender is subscribed | Refused with 4009 (Pusher rule: private and presence only) | Stricter and matches Pusher. |
| HTTP API `auth_timestamp` | Not checked | Must be within 600 seconds, else 401 | Pusher rule, blocks replays. |
| Origin or quota refused | Sends `pusher:error` and leaves the socket open | Sends `pusher:error`, then closes with the same code | Avoids half-open sockets. |
| Empty `channels` list | PHP encodes an empty array as `[]` | `{}` | Clients expect an object. |
| Channel type prefix | `startsWith('private')` (no dash) | `private-`, `presence-`, `cache-` with the dash | Pusher naming. |
| Rate limiting, Redis presence merge, TLS in process | Present | Not built | See README, known gaps. |

## Notes from the run

- The terminate-connections check uses a raw socket. A pusher-js client reconnects by
  itself after the server drops it, so its state looks "connected" again.
- The first draft of the suite had a bug in its own event collector (it ignored events
  that arrived before the test started waiting). It was fixed before the recorded runs.
- All processes started for the runs were stopped. Ports used: 6001 (Reverb), 6002
  (reverb-go), 6010+ (auth server).
