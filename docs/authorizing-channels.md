# Authorizing Channels

Private and presence channels need the server's permission. When Echo joins one, it posts the socket id and the channel name to `/broadcasting/auth`. Your app checks that the user may join and answers with a signature that reverb-go verifies.

## Defining authorization callbacks

Channel rules go in a `routes/channels.go` file, like Laravel's `routes/channels.php`. `Registry.Channel` takes a pattern without the `private-` or `presence-` prefix: `orders.{orderId}` guards `private-orders.5` and `presence-orders.5`. A `{name}` matches one segment without dots, and lands in the params map:

```go
func Channels(registry *reverbgo.Registry) error {
	return registry.Channel("orders.{orderId}", func(ctx context.Context, user any, p map[string]string) (bool, *broadcast.PresenceMember, error) {
		return ownsOrder(ctx, user, p["orderId"]), nil, nil
	})
}
```

A `reverbgo.Callback` returns whether the user may join. A private rule returns `nil` for the member. A returned error is a server fault (500), not a denial. Rules are tried in order, and a channel name with no rule is denied.

## Registering the auth route

`AuthHandlerFrom` builds the `/broadcasting/auth` handler from the bound credentials and registry. Put it inside your session (and CSRF) middleware group, like any browser route:

```go
func routes(app foundation.Application) error {
	if err := Channels(reverbgo.RegistryFrom(app)); err != nil {
		return err
	}
	auth, err := reverbgo.AuthHandlerFrom(app, sessionUser, facades.Log())
	if err != nil {
		return err
	}

	web := facades.Route().Middleware(sessionmiddleware.StartSession(), reverbgo.SocketIDMiddleware())
	web.Group(func(router route.Router) {
		router.Post("/broadcasting/auth", auth.Handle)
	})

	return nil
}
```

The second argument is a `reverbgo.UserResolver`: it returns the signed-in user, or `false` for a guest. The value is handed to your rules unchanged:

```go
// sessionUser is the broadcasting UserResolver.
func sessionUser(ctx contractshttp.Context) (any, bool) {
	id := ctx.Request().Session().Get("auth_user_id")

	return id, id != nil && id != ""
}
```

| Answer | When |
|---|---|
| 200 with `auth` (and `channel_data` for presence) | The rule allowed the user. |
| 422 | A malformed `socket_id` or `channel_name`. |
| 403 | No user, or the rule said no. |
| 500 | The rule returned an error. It is logged once. |

The handler reads a form body or a JSON body. It checks `socket_id` (`^\d+\.\d+$`) and the channel name (`^[-a-zA-Z0-9_=@,.;]{1,200}$`) before signing.

## Testing a rule

`Registry.Authorize` runs the first matching rule for a wire channel name, so you can test rules without HTTP:

```go
func TestOnlyTheOwnerJoins(t *testing.T) {
	registry := reverbgo.NewRegistry()
	if err := registry.Channel("users.{id}", func(_ context.Context, user any, p map[string]string) (bool, *broadcast.PresenceMember, error) {
		return user == p["id"], nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	ok, err := registry.Authorize(context.Background(), "5", "private-users.5")
	if err != nil || !ok.Allowed {
		t.Fatalf("user 5 must join: %v", err)
	}
	no, err := registry.Authorize(context.Background(), "6", "private-users.5")
	if err != nil || no.Allowed {
		t.Fatal("user 6 must not join")
	}
}
```

## Defining channel classes

Laravel's channel classes are not ported. A rule is a function; group related rules in one Go file.
