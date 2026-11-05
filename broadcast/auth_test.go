package broadcast_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/vortechron/reverb-go/broadcast"
)

// Literals come from the Pusher auth-signature docs and were re-checked with PHP hash_hmac.
var creds = broadcast.Credentials{AppID: "3", Key: "278d425bdf160c739803", Secret: "7ad3773142a6692b25b8"}

func TestAuthorizePrivateSignsSocketAndChannel(t *testing.T) {
	got, err := creds.AuthorizePrivate("1234.1234", "private-foobar")
	if err != nil {
		t.Fatal(err)
	}
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

var (
	badSocketIDs = []string{"", "abc", "1.2:x", "1234", "1.2.3", "1.2\n", " 1.2", "1.2:private-a"}
	badChannels  = []string{"", "private-a:b", "private a", "private-a\n", "private-<x>", "a/b", "p\u00e9", strings.Repeat("a", 201)}
)

func TestAuthorizePrivateRejectsUnsafeInput(t *testing.T) {
	for _, id := range badSocketIDs {
		if got, err := creds.AuthorizePrivate(id, "private-a"); err == nil || got.Auth != "" {
			t.Errorf("socket_id %q must be rejected, got %+v", id, got)
		}
	}
	for _, ch := range badChannels {
		if got, err := creds.AuthorizePrivate("1.2", ch); err == nil || got.Auth != "" {
			t.Errorf("channel %q must be rejected, got %+v", ch, got)
		}
	}
}

func TestAuthorizePresenceRejectsUnsafeInput(t *testing.T) {
	m := broadcast.PresenceMember{UserID: 1}
	for _, id := range badSocketIDs {
		if got, err := creds.AuthorizePresence(id, "presence-a", m); err == nil || got.Auth != "" {
			t.Errorf("socket_id %q must be rejected", id)
		}
	}
	for _, ch := range badChannels {
		if got, err := creds.AuthorizePresence("1.2", ch, m); err == nil || got.Auth != "" {
			t.Errorf("channel %q must be rejected", ch)
		}
	}
	unencodable := broadcast.PresenceMember{UserID: 1, UserInfo: make(chan int)}
	if _, err := creds.AuthorizePresence("1.2", "presence-a", unencodable); err == nil {
		t.Error("unencodable user_info must be rejected")
	}
}

func TestAuthorizeAcceptsAllPusherChannelCharacters(t *testing.T) {
	if _, err := creds.AuthorizePrivate("1.2", "private-Az09_-=@,.;"); err != nil {
		t.Fatal(err)
	}
}

func TestParseAuthRequestRejectsBadChannelName(t *testing.T) {
	for _, ch := range badChannels {
		body := "socket_id=1.2&channel_name=" + url.QueryEscape(ch)
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if _, err := broadcast.ParseAuthRequest(r); err == nil {
			t.Errorf("channel %q must be rejected", ch)
		}
	}
}

func TestParseAuthRequestContentTypeIsCaseInsensitive(t *testing.T) {
	for _, ctype := range []string{"Application/JSON", "application/json; charset=utf-8", "APPLICATION/JSON;charset=UTF-8"} {
		r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x",
			strings.NewReader(`{"socket_id":"1.2","channel_name":"private-a"}`))
		r.Header.Set("Content-Type", ctype)
		if got, err := broadcast.ParseAuthRequest(r); err != nil || got.ChannelName != "private-a" {
			t.Errorf("%q: got %+v err=%v", ctype, got, err)
		}
	}
	// A broken media type falls back to the form parser, which finds no fields.
	r := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x", strings.NewReader(`{"socket_id":"1.2"}`))
	r.Header.Set("Content-Type", ";;;")
	if _, err := broadcast.ParseAuthRequest(r); err == nil {
		t.Fatal("expected an error")
	}
}

func TestParseAuthRequestCapsBothBodyTypesAt64KB(t *testing.T) {
	pad := strings.Repeat("a", 70<<10)
	form := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x",
		strings.NewReader("socket_id=1.2&channel_name=private-a&pad="+pad))
	form.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := broadcast.ParseAuthRequest(form); err == nil {
		t.Error("an oversized form body must be rejected")
	}
	js := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/x",
		strings.NewReader(`{"socket_id":"1.2","channel_name":"private-a","pad":"`+pad+`"}`))
	js.Header.Set("Content-Type", "application/json")
	if _, err := broadcast.ParseAuthRequest(js); err == nil {
		t.Error("an oversized JSON body must be rejected")
	}
}
