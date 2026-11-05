# Conformance results

Date: 2026-11-05. The same suite (`conformance/suite/*.test.mjs`, 66 checks) ran
against both servers on this Mac. Both passed all 66. The first 61 checks passed on both
before the review fixes; 5 checks with the real pusher-php-server were added after.

| Target | How it was started | Result |
|---|---|---|
| Laravel Reverb v1.11.1 (PHP 8.4.25) | `run-reverb.sh`: `php artisan reverb:start --host=127.0.0.1 --port=6001` from a Laravel 12 app with laravel/reverb installed, env overrides only, `APP_CONFIG_CACHE=/tmp/reverb-conf-missing-config.php`, no file edited | 66 passed, 0 failed |
| reverb-go (this repo) | `run-reverb-go.sh`: built binary on `127.0.0.1:6002` | 66 passed, 0 failed (run three times, same result) |

Clients used (Node v22.14.0): pusher-js 8.6.0, laravel-echo 1.19.0 (alias `echo1`),
laravel-echo 2.6.3 (alias `echo2`), ws 8.22.1.

## What is checked

Real PHP publisher (5 checks, `suite/php.test.mjs`): `conformance/php/pusher.php` loads
pusher-php-server 7.3.0 from the werewolf app's `vendor/` (read only) and runs `trigger()`
(Laravel-style `App\Events\...` name and array payload, `socket_id` exclusion, many
channels), `triggerBatch()`, `getChannels()`, and `authorizeChannel()` /
`authorizePresenceChannel()` whose auth strings the server must accept. These tests skip
with a message when `php` or the vendor folder is missing (for example in CI). They ran and
passed against both servers on this Mac.

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
| Presence subscribe without `channel_data` | Accepted if signed, then **every** member list is sent empty while that socket is on the channel | Refused with 4009 | One client could blank the member list for everyone. Pusher also requires a user id. |
| Channel prefix without dash (`privatefoo`, `presencefoo`, `cachefoo`) | Treated as private, presence, cache | Same (matched after review) | Safer: such names need auth. |
| Missing `Origin` header when origins are restricted | Refused (host is empty, no pattern matches) | Same | Already matched, now covered by a test. |
| `auth_key` on API calls | Not compared | Must equal the app key, else 401 | Pusher rule. |
| Rate limiting | Optional, off by default, 60 messages per 60 s, error 4301 | Same option names (`REVERB_APP_RATE_LIMIT*`), **on by default**, 600 per 60 s, error 4301 | Abuse protection by default. |
| Channels per connection, presence members, `channel_data` size | No limit | 100 channels (4100), 100 members (4100), 5000 bytes (4200), all configurable | Abuse protection. Pusher's own limits are similar. |
| Event batch size, channels per event, event name length | No limit | 10 events, 100 channels, 200 characters, answered with 422 (Reverb's validation status; Pusher uses 400) | Pusher limits. Laravel never sends more. |
| API request timing | No deadline | Body must arrive within 10 s (408). WebSockets are not affected | Slow-body attack. |
| Order of `ids` in a presence list | Join order | Not ordered | Keeps join and leave O(1). pusher-js does not depend on it. |
| Redis messages | PHP-serialized, unsigned | JSON signed with HMAC (`REVERB_SCALING_SECRET`), unsigned messages are dropped | Anyone who can write to Redis could otherwise inject events. |
| Rate limiting of Redis presence merge, TLS in process | Present | Not built | See README, known gaps. |

## Notes from the run

- The terminate-connections check uses a raw socket. A pusher-js client reconnects by
  itself after the server drops it, so its state looks "connected" again.
- `run-reverb.sh` now `exec`s php so its trap stops the real Reverb process (an earlier
  version left one running after a run; it was found and stopped).
- The first draft of the suite had a bug in its own event collector (it ignored events
  that arrived before the test started waiting). It was fixed before the recorded runs.
- All processes started for the runs were stopped. Ports used: 6001 (Reverb), 6002
  (reverb-go), 6010+ (auth server).
