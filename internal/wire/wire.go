// Package wire builds the JSON frames of the Pusher protocol, in the same
// shapes Laravel Reverb sends.
package wire

import (
	"bytes"
	"encoding/json"
)

// Error codes, as Reverb sends them.
const (
	CodeAppNotFound    = 4001
	CodeOverQuota      = 4004
	CodeUnauthorized   = 4009
	CodeInvalidMessage = 4200
	CodePongTimeout    = 4201
	CodeRateLimit      = 4301
)

type frame struct {
	Event   string `json:"event"`
	Data    string `json:"data,omitempty"`
	Channel string `json:"channel,omitempty"`
}

func encode(f frame) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(f) // a struct of strings cannot fail to encode
	return bytes.TrimRight(buf.Bytes(), "\n")
}

func jsonString(v any) string {
	raw, _ := json.Marshal(v) // only used with plain maps and numbers
	return string(raw)
}

// Established is the first frame of every connection.
func Established(socketID string, activityTimeoutSeconds int) []byte {
	return encode(frame{Event: "pusher:connection_established", Data: jsonString(
		struct {
			SocketID        string `json:"socket_id"`
			ActivityTimeout int    `json:"activity_timeout"`
		}{socketID, activityTimeoutSeconds})})
}

// Pong answers pusher:ping.
func Pong() []byte { return encode(frame{Event: "pusher:pong"}) }

// Ping asks an idle client to prove it is alive.
func Ping() []byte { return encode(frame{Event: "pusher:ping"}) }

// Error is a pusher:error frame.
func Error(code int, message string) []byte {
	return encode(frame{Event: "pusher:error", Data: jsonString(
		struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{code, message})})
}

// Internal builds a pusher_internal:<name> frame. data must be a JSON string.
func Internal(name, channel, data string) []byte {
	return encode(frame{Event: "pusher_internal:" + name, Data: data, Channel: channel})
}

// CacheMiss tells a cache channel subscriber that nothing is cached yet.
func CacheMiss(channel string) []byte {
	return encode(frame{Event: "pusher:cache_miss", Channel: channel})
}

// Event is an application event delivered to subscribers. data is a JSON string.
func Event(name, channel, data string) []byte {
	return encode(frame{Event: name, Data: data, Channel: channel})
}
