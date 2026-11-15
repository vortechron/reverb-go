# Broadcasting Events

In many modern web applications, WebSockets push live updates to the browser. When an order ships, the server broadcasts an event, and the open order page updates without a reload.

reverb-go is the WebSocket server. On the Go side of your app, the `goravel` package publishes events to it the way Laravel's `broadcast()` helper does. A Laravel app keeps using its own broadcasting, pointed at reverb-go.

## Concept overview

1. Your server publishes an event to one or more channels through the Pusher HTTP API.
2. reverb-go delivers it to every browser subscribed to those channels.
3. Laravel Echo in the browser [listens](receiving-broadcasts) for the event by name.

Public channels need no auth. Private and presence channels need a [channel rule](authorizing-channels) that says who may join.

## Broadcasting an event

`reverbgo.BroadcasterFrom(app).Broadcast` sends an event to channels:

```go
func shipped(ctx context.Context, order Order) error {
	return reverbgo.BroadcasterFrom(facades.App()).Broadcast(ctx,
		[]string{fmt.Sprintf("private-orders.%d", order.ID)},
		`App\Events\OrderShipmentStatusUpdated`,
		map[string]any{"order": order},
	)
}
```

A Go app must send exactly what Laravel would, or Echo listeners will not match:

- **Channel names keep their type prefix on the wire**: `private-orders.5`, `presence-chat.1`. Echo adds the prefix for you. Go code must write it.
- **The event name** of a Laravel event class is its full class name, for example `App\Events\OrderShipmentStatusUpdated`. Echo's `.listen('OrderShipmentStatusUpdated')` matches that name.
- **The payload** is the event's public properties at the top level, or the array `broadcastWith()` returns. Laravel adds nothing else.

There are no broadcast event classes in Go. The channel list, the name and the payload are the arguments of `Broadcast`.

### Broadcast name

Laravel's `broadcastAs()` sends a custom name. Send that string as the event name:

```go
return reverbgo.BroadcasterFrom(facades.App()).Broadcast(ctx,
	[]string{"orders"}, "server.created", map[string]any{"order": order})
```

In the browser, listen with a leading dot, so Echo does not add the `App\Events` namespace: `.listen('.server.created', ...)`.

### Broadcast data

The payload is any value that encodes to a JSON object. A string payload is sent as it is.

## Only to others

When a user's own action causes the event, their browser tab often has the change already. `ToOthers()` leaves out the tab that sent the request, like Laravel's `toOthers()`:

```go
func shippedToOthers(ctx contractshttp.Context, order Order) error {
	return reverbgo.BroadcasterFrom(facades.App()).Broadcast(ctx.Context(),
		[]string{fmt.Sprintf("private-orders.%d", order.ID)},
		`App\Events\OrderShipmentStatusUpdated`,
		map[string]any{"order": order},
		reverbgo.ToOthers(),
	)
}
```

Echo sends the tab's socket id in the `X-Socket-Id` header of every request. `reverbgo.SocketIDMiddleware()` keeps it in the request context, so put the middleware on the routes that broadcast. Without a socket id, everyone gets the event. `reverbgo.ExceptSocket(id)` leaves out one named connection.

## Drivers

`BROADCAST_CONNECTION` picks the publisher, as in Laravel:

| Driver | Does |
|---|---|
| `reverb` | Sends the event over the Pusher HTTP API to `REVERB_HOST`. |
| `log` | Logs the event. |
| `null` | Drops the event. The default. |

## Not ported

Laravel's `ShouldBroadcast` queueing, `broadcastWhen()`, `ShouldDispatchAfterCommit`, anonymous events through the `Broadcast` facade, rescuing failed broadcasts and model broadcasting are not in the Go package. Call `Broadcast` from your own code, after the transaction commits and from a queue job when you need it. See [Differences from Laravel](differences).
