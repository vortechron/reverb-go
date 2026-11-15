# Receiving Broadcasts

The browser side is unchanged from Laravel: [Laravel Echo](https://github.com/laravel/echo) with pusher-js connects to reverb-go as it connects to Reverb.

## Installing Echo

```shell
npm install --save-dev laravel-echo pusher-js
```

```ts
import Echo from 'laravel-echo';

import Pusher from 'pusher-js';
window.Pusher = Pusher;

window.Echo = new Echo({
    broadcaster: 'reverb',
    key: import.meta.env.VITE_REVERB_APP_KEY,
    wsHost: import.meta.env.VITE_REVERB_HOST,
    wsPort: import.meta.env.VITE_REVERB_PORT ?? 80,
    wssPort: import.meta.env.VITE_REVERB_PORT ?? 443,
    forceTLS: (import.meta.env.VITE_REVERB_SCHEME ?? 'https') === 'https',
    enabledTransports: ['ws', 'wss'],
});
```

Set `VITE_REVERB_APP_KEY`, `VITE_REVERB_HOST`, `VITE_REVERB_PORT` and `VITE_REVERB_SCHEME` to reverb-go's key and public address. Echo posts to `/broadcasting/auth` for private and presence channels, which the [auth route](authorizing-channels#registering-the-auth-route) answers.

## Listening for events

```ts
Echo.private(`orders.${orderId}`)
    .listen('OrderShipmentStatusUpdated', (e) => {
        console.log(e.order);
    });
```

`Echo.private('orders.5')` joins `private-orders.5`. `listen('OrderShipmentStatusUpdated')` matches the event name `App\Events\OrderShipmentStatusUpdated`, because Echo adds the `App\Events` namespace. For a custom name, add a leading dot: `.listen('.server.created', ...)`.

`Echo.channel('orders')` joins a public channel.

## Leaving a channel

```ts
Echo.leave(`orders.${orderId}`);
```

## A working page

The repository's `examples/echo-page` is a small page that connects with Echo and prints the events it receives. Its `server.mjs` serves the page and the config.
