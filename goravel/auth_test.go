package goravel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	contractshttp "github.com/goravel/framework/contracts/http"
	mockshttp "github.com/goravel/framework/mocks/http"

	"github.com/vortechron/reverb-go/broadcast"
)

var testCreds = broadcast.Credentials{AppID: "app", Key: "key", Secret: "secret"}

type recorded struct {
	status int
	body   any
}

type fakeLog struct{ lines []string }

func (l *fakeLog) Errorf(format string, args ...any) {
	l.lines = append(l.lines, fmt.Sprintf(format, args...))
}

// serve runs the handler against a Goravel context mock built on a real
// httptest request, and returns what the handler wrote.
func serve(t *testing.T, h *AuthHandler, r *http.Request) recorded {
	t.Helper()
	var out recorded
	ctx := mockshttp.NewContext(t)
	req := mockshttp.NewContextRequest(t)
	resp := mockshttp.NewContextResponse(t)
	ctx.EXPECT().Request().Return(req).Maybe()
	ctx.EXPECT().Response().Return(resp).Maybe()
	ctx.EXPECT().Context().Return(r.Context()).Maybe()
	req.EXPECT().Origin().Return(r).Maybe()
	resp.EXPECT().Json(mock.Anything, mock.Anything).RunAndReturn(func(code int, obj any) contractshttp.AbortableResponse {
		out = recorded{status: code, body: obj}

		return mockshttp.NewAbortableResponse(t)
	}).Maybe()
	h.Handle(ctx) //nolint:contextcheck // the Goravel context carries the request context

	return out
}

func formRequest(socket, channel string) *http.Request {
	form := url.Values{"socket_id": {socket}, "channel_name": {channel}}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/broadcasting/auth", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return r
}

func newHandler(t *testing.T, user UserResolver, log Logger) *AuthHandler {
	t.Helper()
	reg := NewRegistry()
	require.NoError(t, reg.Channel("users.{id}", func(_ context.Context, u any, p map[string]string) (bool, *broadcast.PresenceMember, error) {
		return u == p["id"], nil, nil
	}))
	require.NoError(t, reg.Channel("room.{id}", func(_ context.Context, u any, _ map[string]string) (bool, *broadcast.PresenceMember, error) {
		return true, &broadcast.PresenceMember{UserID: u, UserInfo: map[string]any{"name": "Ana"}}, nil
	}))
	require.NoError(t, reg.Channel("boom", func(context.Context, any, map[string]string) (bool, *broadcast.PresenceMember, error) {
		return false, nil, errors.New("db down")
	}))
	h, err := NewAuthHandler(AuthConfig{Credentials: testCreds, Registry: reg, User: user, Log: log})
	require.NoError(t, err)

	return h
}

func signedIn(id string) UserResolver {
	return func(contractshttp.Context) (any, bool) { return id, true }
}

func TestAuthPrivateAllowedReturnsSignature(t *testing.T) {
	h := newHandler(t, signedIn("5"), nil)

	got := serve(t, h, formRequest("123.456", "private-users.5"))

	assert.Equal(t, http.StatusOK, got.status)
	want, err := testCreds.AuthorizePrivate("123.456", "private-users.5")
	require.NoError(t, err)
	assert.Equal(t, want, got.body)
}

func TestAuthAcceptsJSONBody(t *testing.T) {
	h := newHandler(t, signedIn("5"), nil)
	r := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/broadcasting/auth",
		strings.NewReader(`{"socket_id":"1.2","channel_name":"private-users.5"}`))
	r.Header.Set("Content-Type", "application/json")

	assert.Equal(t, http.StatusOK, serve(t, h, r).status)
}

func TestAuthDenied(t *testing.T) {
	cases := map[string]UserResolver{
		"callback says no": signedIn("9"),
		"no user":          func(contractshttp.Context) (any, bool) { return nil, false },
	}
	for name, user := range cases {
		t.Run(name, func(t *testing.T) {
			got := serve(t, newHandler(t, user, nil), formRequest("1.2", "private-users.5"))

			assert.Equal(t, http.StatusForbidden, got.status)
		})
	}
}

func TestAuthUnknownChannelIsForbidden(t *testing.T) {
	got := serve(t, newHandler(t, signedIn("5"), nil), formRequest("1.2", "private-nothing.1"))

	assert.Equal(t, http.StatusForbidden, got.status)
}

func TestAuthBadInputIs422(t *testing.T) {
	h := newHandler(t, signedIn("5"), nil)

	for _, r := range []*http.Request{formRequest("nope", "private-users.5"), formRequest("1.2", "")} {
		assert.Equal(t, http.StatusUnprocessableEntity, serve(t, h, r).status)
	}
}

func TestAuthPresenceReturnsChannelData(t *testing.T) {
	h := newHandler(t, signedIn("5"), nil)

	got := serve(t, h, formRequest("1.2", "presence-room.1"))

	require.Equal(t, http.StatusOK, got.status)
	resp, ok := got.body.(broadcast.AuthResponse)
	require.True(t, ok)
	assert.JSONEq(t, `{"user_id":"5","user_info":{"name":"Ana"}}`, resp.ChannelData)
	want, err := testCreds.AuthorizePresence("1.2", "presence-room.1", broadcast.PresenceMember{UserID: "5", UserInfo: map[string]any{"name": "Ana"}})
	require.NoError(t, err)
	assert.Equal(t, want.Auth, resp.Auth)
}

func TestAuthCallbackErrorIs500AndLoggedOnce(t *testing.T) {
	log := &fakeLog{}
	h := newHandler(t, signedIn("5"), log)

	got := serve(t, h, formRequest("1.2", "private-boom"))

	assert.Equal(t, http.StatusInternalServerError, got.status)
	assert.Len(t, log.lines, 1)
	assert.Contains(t, log.lines[0], "db down", "the cause is logged, not shown to the client")
	assert.NotContains(t, got.body, "db down")
}

func TestNewAuthHandlerValidatesConfig(t *testing.T) {
	_, err := NewAuthHandler(AuthConfig{Credentials: testCreds, User: signedIn("1")})
	assert.Error(t, err, "registry required")
	_, err = NewAuthHandler(AuthConfig{Credentials: testCreds, Registry: NewRegistry()})
	assert.Error(t, err, "user resolver required")
	_, err = NewAuthHandler(AuthConfig{Registry: NewRegistry(), User: signedIn("1")})
	assert.Error(t, err, "credentials required")
}
