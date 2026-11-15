# Presence Channels

Presence channels add awareness of who is subscribed: a chat room can show who is online.

## Authorizing presence channels

A presence rule returns the member that joins. The member's `UserID` and `UserInfo` go to every other subscriber:

```go
return registry.Channel("chat.{roomId}", func(ctx context.Context, user any, p map[string]string) (bool, *broadcast.PresenceMember, error) {
	u, ok := findUser(ctx, user)
	if !ok || !inRoom(ctx, user, p["roomId"]) {
		return false, nil, nil
	}

	return true, &broadcast.PresenceMember{UserID: u.ID, UserInfo: map[string]any{"name": u.Name}}, nil
})
```

The rule guards `presence-chat.1` and also `private-chat.1`. A rule that allows a presence channel but returns no member fails with `reverbgo.ErrNoPresenceMember`: that is a bug in the rule, not a denial.

`UserID` is kept as given, so an int stays an int, as Laravel sends it.

## Joining presence channels

```ts
Echo.join(`chat.${roomId}`)
    .here((users) => {
        // everyone in the room now
    })
    .joining((user) => {
        console.log(user.name);
    })
    .leaving((user) => {
        console.log(user.name);
    });
```

A user with two open tabs counts once: `member_added` fires on the first socket and `member_removed` on the last.

## Broadcasting to presence channels

Broadcast to the `presence-` name:

```go
return reverbgo.BroadcasterFrom(facades.App()).Broadcast(ctx,
	[]string{"presence-chat." + roomID}, `App\Events\NewMessage`, map[string]any{"message": message},
	reverbgo.ToOthers())
```

## Limits

A presence channel holds at most `REVERB_APP_MAX_PRESENCE_MEMBERS` distinct users (default 100), and `channel_data` is at most `REVERB_APP_MAX_CHANNEL_DATA_SIZE` bytes (default 5000). A presence subscribe without a user id is refused with 4009.

> [!WARNING]
> With [Redis scaling](production#scaling), presence lists are per node. A presence channel shows only the users connected to the same node.
