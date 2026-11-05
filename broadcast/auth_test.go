package broadcast_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vortechron/reverb-go/broadcast"
)

// Literals come from the Pusher auth-signature docs and were re-checked with PHP hash_hmac.
var creds = broadcast.Credentials{AppID: "3", Key: "278d425bdf160c739803", Secret: "7ad3773142a6692b25b8"}

func TestAuthorizePrivateSignsSocketAndChannel(t *testing.T) {
	got := creds.AuthorizePrivate("1234.1234", "private-foobar")
	want := "278d425bdf160c739803:58df8b0c36d6982b82c3ecf6b4662e34fe8c25bba48f5369f135bf843651c3a4"
	if got.Auth != want {
		t.Fatalf("auth = %q, want %q", got.Auth, want)
	}
	if got.ChannelData != "" {
		t.Fatalf("private auth must not carry channel_data, got %q", got.ChannelData)
	}
}

func TestAuthorizePresenceSignsChannelData(t *testing.T) {
	member := broadcast.PresenceMember{UserID: 10, UserInfo: map[string]string{"name": "Mr. Pusher"}}
	got, err := creds.AuthorizePresence("1234.1234", "presence-foobar", member)
	if err != nil {
		t.Fatal(err)
	}
	wantData := `{"user_id":10,"user_info":{"name":"Mr. Pusher"}}`
	if got.ChannelData != wantData {
		t.Fatalf("channel_data = %q, want %q", got.ChannelData, wantData)
	}
	want := "278d425bdf160c739803:afaed3695da2ffd16931f457e338e6c9f2921fa133ce7dac49f529792be6304c"
	if got.Auth != want {
		t.Fatalf("auth = %q, want %q", got.Auth, want)
	}
}

func TestAuthorizePresenceRejectsMissingUserID(t *testing.T) {
	if _, err := creds.AuthorizePresence("1.2", "presence-x", broadcast.PresenceMember{}); err == nil {
		t.Fatal("expected an error when user_id is missing")
	}
}

func TestParseAuthRequestAcceptsFormBody(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/broadcasting/auth",
		strings.NewReader("socket_id=1.2&channel_name=private-a"))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	got, err := broadcast.ParseAuthRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.SocketID != "1.2" || got.ChannelName != "private-a" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseAuthRequestAcceptsJSONBody(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/api/broadcasting/auth",
		strings.NewReader(`{"socket_id":"1.2","channel_name":"presence-b"}`))
	r.Header.Set("Content-Type", "application/json")
	got, err := broadcast.ParseAuthRequest(r)
	if err != nil {
		t.Fatal(err)
	}
	if got.SocketID != "1.2" || got.ChannelName != "presence-b" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseAuthRequestRejectsBadInput(t *testing.T) {
	cases := map[string]struct{ ctype, body string }{
		"empty json fields": {"application/json", `{}`},
		"broken json":       {"application/json", `{`},
		"form missing name": {"application/x-www-form-urlencoded", "socket_id=1.2"},
		"bad socket id":     {"application/x-www-form-urlencoded", "socket_id=abc&channel_name=private-a"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", strings.NewReader(c.body))
			r.Header.Set("Content-Type", c.ctype)
			if _, err := broadcast.ParseAuthRequest(r); err == nil {
				t.Fatal("expected an error")
			}
		})
	}
}

func TestSocketIDFromRequestReadsEchoHeader(t *testing.T) {
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", nil)
	r.Header.Set("X-Socket-Id", "12.34")
	if got := broadcast.SocketIDFromRequest(r); got != "12.34" {
		t.Fatalf("got %q", got)
	}
}
