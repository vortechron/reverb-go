# Running the Server

Start the binary with its environment:

```shell
REVERB_APP_ID=app REVERB_APP_KEY=key REVERB_APP_SECRET=secret ./reverb-go
# listens on 0.0.0.0:8080 ; health check: GET /up
```

Change the address with `REVERB_SERVER_HOST` and `REVERB_SERVER_PORT`.

## The routes

| Route | Serves |
|---|---|
| `GET /app/{key}` | The WebSocket upgrade. `protocol=7` in the query is ignored, like Reverb. |
| `POST /apps/{id}/events`, `/apps/{id}/batch_events` | Publish events. |
| `GET /apps/{id}/channels`, `/channels/{name}`, `/channels/{name}/users`, `/connections` | Channel and connection info. |
| `POST /apps/{id}/users/{user}/terminate_connections` | Close a user's connections. |
| `GET /up` | The health check. It answers 503 while the Redis bus is down. |

With `REVERB_SERVER_PATH` set, every route sits under that prefix.

## Debugging

The server logs to stderr in Go's `log/slog` text format: the listen address at start, the error that stopped it, and Redis bus problems such as dropped messages. There is no debug mode that prints every frame. To watch the traffic, open the browser's network panel on the WebSocket, or call the HTTP API's `/channels` and `/connections` routes.

## Restarting

On `SIGINT` or `SIGTERM`, reverb-go closes the WebSocket sessions, stops accepting new connections and shuts the HTTP server down within 10 seconds. Clients reconnect by themselves. Restart the process to apply new settings: there is no `reverb:restart` command.
