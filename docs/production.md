# Running in Production

## Web server

Run reverb-go behind a reverse proxy that terminates TLS, such as Caddy, nginx or a Kubernetes ingress. The proxy must pass the WebSocket upgrade for `/app/` and plain HTTP for `/apps/`.

The HTTP API has a 10 second read deadline per request (408 on a slow body), set per request so WebSockets are not cut. Give your proxy a longer timeout for WebSocket connections than the ping interval.

## Ports

reverb-go listens on one port for WebSockets and the HTTP API. Expose only the proxy's port to the internet. Your app's server reaches the HTTP API on the private address.

## Open files

Every WebSocket is an open file. On Linux, raise the open file limit of the process (`ulimit -n`, or `LimitNOFILE` in a systemd unit) above the number of connections you expect.

## Process management

reverb-go is one binary. Run it under systemd, a container runtime or Kubernetes, and restart it when it stops. `GET /up` is the health check.

## Event loop

There is no event loop to choose. Each connection has one read goroutine and one write goroutine, and a bounded send queue of 256 frames. A client that cannot keep up is disconnected when its queue is full, so a slow client never blocks the others.

## Scaling

With `REVERB_SCALING_ENABLED=true`, each node publishes events (whispers included) and terminate requests to a Redis channel, and every other node delivers them to its own clients. Each node delivers locally first, then queues a signed message for Redis. A slow Redis never blocks a client or an API call: a full queue drops the message and logs it.

- Redis must have a password or ACL, and should not be reachable from the internet. Every message is signed with `REVERB_SCALING_SECRET`, and nodes drop unsigned or badly signed ones.
- If Redis is not reachable at start, the server stops with an error. If the subscription drops later, the node retries with backoff (500 ms up to 30 s), and `/up` answers 503 until it is back.

> [!WARNING]
> A reverb-go node cannot join a PHP Reverb cluster: Reverb publishes PHP-serialized objects, reverb-go signed JSON. Replace all nodes at once.

> [!WARNING]
> Presence lists, channel info and connection counts are per node. In a cluster, a presence channel shows only the users connected to the same node. Use one node, or sticky routing per channel, if you rely on presence.
