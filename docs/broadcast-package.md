# The broadcast Package

`github.com/vortechron/reverb-go/broadcast` is the framework-free core. It publishes events over the Pusher HTTP API and signs channel auth answers. It works against reverb-go and Laravel Reverb.

## Publishing

```go
pub := broadcast.NewPublisher(broadcast.Config{Credentials: creds, BaseURL: "http://127.0.0.1:8080"})

err := pub.Trigger(ctx, []string{"private-orders.5"}, `App\Events\OrderShipmentStatusUpdated`,
	map[string]any{"order": map[string]any{"id": 5, "status": "shipped"}},
	broadcast.WithSocketID(broadcast.SocketIDFromRequest(r)))
if err != nil {
	return err
}

return pub.TriggerBatch(ctx, []broadcast.Event{
	{Channel: "news", Name: "PostPublished", Data: map[string]any{"id": 1}},
})
```

`creds` is a `broadcast.Credentials{AppID, Key, Secret}`. More than 100 channels are split into several requests. A string `Data` is sent as it is, anything else as JSON. `broadcast.Config.Client` replaces the HTTP client, which has a 10 second timeout by default.

`broadcast.WithSocketID` leaves out one connection. `broadcast.SocketIDFromRequest(r)` reads Echo's `X-Socket-Id` header.

## Answering /broadcasting/auth

Without Goravel, write the auth handler yourself:

```go
func authHandler(w http.ResponseWriter, r *http.Request) {
	req, err := broadcast.ParseAuthRequest(r) // validates socket_id and channel_name
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)

		return
	}
	user, ok := currentUser(r)
	if !ok || !mayJoin(user, req.ChannelName) {
		http.Error(w, "forbidden", http.StatusForbidden)

		return
	}

	var resp broadcast.AuthResponse
	if len(req.ChannelName) > 9 && req.ChannelName[:9] == "presence-" {
		resp, err = creds.AuthorizePresence(req.SocketID, req.ChannelName,
			broadcast.PresenceMember{UserID: user.ID, UserInfo: map[string]any{"name": user.Name}})
	} else {
		resp, err = creds.AuthorizePrivate(req.SocketID, req.ChannelName)
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)

		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
```

`ParseAuthRequest` reads a form body or a JSON body. `AuthorizePrivate` and `AuthorizePresence` check the socket id and the channel name before signing, and return a `broadcast.AuthResponse` with `auth` and, for presence, `channel_data`.

## Building blocks

| Function | Does |
|---|---|
| `RequestSignature(secret, method, path, params, body)` | The Pusher HTTP API signature. |
| `HMACHex(secret, message)` | HMAC-SHA256 in hex. |
| `ValidSocketID(id)` | `^\d+\.\d+$` |
| `ValidChannelName(name)` | `^[-a-zA-Z0-9_=@,.;]{1,200}$` |
