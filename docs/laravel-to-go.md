# Laravel to Go

This table maps Laravel Reverb and Laravel's broadcasting API to reverb-go. `reverbgo` is `github.com/vortechron/reverb-go/goravel`.

## The server

| Laravel | Go |
|---|---|
| `composer require laravel/reverb`, `php artisan install:broadcasting` | `go install github.com/vortechron/reverb-go/cmd/reverb-go@latest` |
| `php artisan reverb:start --host --port` | `./reverb-go` with `REVERB_SERVER_HOST`, `REVERB_SERVER_PORT` |
| `php artisan reverb:restart` | Restart the process (`SIGTERM` shuts it down cleanly) |
| `config/reverb.php` apps, `REVERB_APP_*` | The same env names, or `REVERB_APPS_FILE` |
| `allowed_origins` | `REVERB_ALLOWED_ORIGINS` |
| `REVERB_SCALING_ENABLED` with Redis | The same, plus the required `REVERB_SCALING_SECRET` |
| `/up` health route | `GET /up` |

## Broadcasting from Go

| Laravel | Go |
|---|---|
| `config/broadcasting.php` | `config.Add("broadcasting", reverbgo.Config(config.Env))` |
| `BROADCAST_CONNECTION=reverb`, `log`, `null` | The same drivers |
| `broadcast(new Event(...))` | `reverbgo.BroadcasterFrom(app).Broadcast(ctx, channels, name, payload)` |
| `->toOthers()` | `reverbgo.ToOthers()` with `reverbgo.SocketIDMiddleware()` |
| `broadcastOn()` | the `channels` argument, with the `private-` or `presence-` prefix |
| `broadcastAs()` | the `name` argument |
| `broadcastWith()` | the `payload` argument |
| `Broadcast::channel('orders.{orderId}', fn)` in `routes/channels.php` | `reverbgo.RegistryFrom(app).Channel("orders.{orderId}", callback)` |
| `Broadcast::routes()` (`/broadcasting/auth`) | `reverbgo.AuthHandlerFrom(app, userResolver, log)` and `router.Post("/broadcasting/auth", auth.Handle)` |
| a presence callback returning `['id' => ..., 'name' => ...]` | `&broadcast.PresenceMember{UserID: ..., UserInfo: ...}` |

## Without a framework

| pusher-php-server | Go |
|---|---|
| `$pusher->trigger($channels, $event, $data, ['socket_id' => $id])` | `pub.Trigger(ctx, channels, event, data, broadcast.WithSocketID(id))` |
| `$pusher->triggerBatch($batch)` | `pub.TriggerBatch(ctx, events)` |
| `$pusher->authorizeChannel($channel, $socketId)` | `creds.AuthorizePrivate(socketID, channel)` |
| `$pusher->authorizePresenceChannel($channel, $socketId, $userId, $userInfo)` | `creds.AuthorizePresence(socketID, channel, broadcast.PresenceMember{...})` |
