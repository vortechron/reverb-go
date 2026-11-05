package wire_test

import (
	"testing"

	"github.com/vortechron/reverb-go/internal/wire"
)

func TestMessagesMatchReverbShapes(t *testing.T) {
	cases := map[string]struct{ got, want string }{
		"established": {
			string(wire.Established("123.456", 30)),
			`{"event":"pusher:connection_established","data":"{\"socket_id\":\"123.456\",\"activity_timeout\":30}"}`,
		},
		"pong": {string(wire.Pong()), `{"event":"pusher:pong"}`},
		"ping": {string(wire.Ping()), `{"event":"pusher:ping"}`},
		"error": {
			string(wire.Error(4009, "Connection is unauthorized")),
			`{"event":"pusher:error","data":"{\"code\":4009,\"message\":\"Connection is unauthorized\"}"}`,
		},
		"succeeded": {
			string(wire.Internal("subscription_succeeded", "public-a", "{}")),
			`{"event":"pusher_internal:subscription_succeeded","data":"{}","channel":"public-a"}`,
		},
		"cache miss": {string(wire.CacheMiss("cache-a")), `{"event":"pusher:cache_miss","channel":"cache-a"}`},
		"app event": {
			string(wire.Event(`App\Events\X`, "private-a", `{"a":1}`)),
			`{"event":"App\\Events\\X","data":"{\"a\":1}","channel":"private-a"}`,
		},
	}
	for name, c := range cases {
		if c.got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, c.got, c.want)
		}
	}
}

func TestErrorCodes(t *testing.T) {
	if wire.CodeAppNotFound != 4001 || wire.CodeOverQuota != 4004 || wire.CodeUnauthorized != 4009 ||
		wire.CodeInvalidMessage != 4200 || wire.CodePongTimeout != 4201 || wire.CodeRateLimit != 4301 {
		t.Fatal("error codes must match Reverb")
	}
}
