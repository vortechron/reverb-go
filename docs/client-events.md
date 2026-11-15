# Client Events

A client event (a "whisper") goes from one browser to the others on a channel without touching your server, for example a "typing" indicator:

```ts
Echo.private(`chat.${roomId}`)
    .whisper('typing', {
        name: this.user.name
    });

Echo.private(`chat.${roomId}`)
    .listenForWhisper('typing', (e) => {
        console.log(e.name);
    });
```

reverb-go relays `client-*` events with these rules:

- Only on private and presence channels. A whisper on a public channel is refused with 4009, as Pusher does.
- Only from a socket subscribed to the channel.
- Never echoed back to the sender.
- On a presence channel, the sender's `user_id` is added.
- Never stored as the cached event of a cache channel.

`REVERB_APP_ACCEPT_CLIENT_EVENTS_FROM` controls who may send them: `members` (the default), `all`, or `none` to turn client events off. Client messages count against the [rate limit](configuration#per-app-limits).
