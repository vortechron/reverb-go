package goravel

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	mockshttp "github.com/goravel/framework/mocks/http"

	"github.com/vortechron/reverb-go/broadcast"
)

// capture is a fake Pusher HTTP API. It records the JSON bodies it receives.
type capture struct {
	srv    *httptest.Server
	bodies []map[string]any
	status int
}

func newCapture(t *testing.T) *capture {
	t.Helper()
	c := &capture{status: http.StatusOK}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		c.bodies = append(c.bodies, body)
		w.WriteHeader(c.status)
		_, _ = w.Write([]byte("{}"))
	}))
	t.Cleanup(c.srv.Close)

	return c
}

func (c *capture) publisher() *broadcast.Publisher {
	return broadcast.NewPublisher(broadcast.Config{Credentials: testCreds, BaseURL: c.srv.URL})
}

func TestBroadcastSendsEventToChannels(t *testing.T) {
	api := newCapture(t)
	b := NewBroadcaster(api.publisher())

	err := b.Broadcast(context.Background(), []string{"notes"}, `App\Events\NoteCreated`, map[string]any{"id": 1})

	require.NoError(t, err)
	require.Len(t, api.bodies, 1)
	assert.Equal(t, `App\Events\NoteCreated`, api.bodies[0]["name"])
	assert.Equal(t, []any{"notes"}, api.bodies[0]["channels"])
	assert.JSONEq(t, `{"id":1}`, api.bodies[0]["data"].(string))
	assert.NotContains(t, api.bodies[0], "socket_id")
}

func TestBroadcastToOthersUsesSocketIDFromContext(t *testing.T) {
	api := newCapture(t)
	b := NewBroadcaster(api.publisher())
	ctx := WithSocketID(context.Background(), "11.22")

	require.NoError(t, b.Broadcast(ctx, []string{"notes"}, "E", nil, ToOthers()))
	require.NoError(t, b.Broadcast(ctx, []string{"notes"}, "E", nil))

	assert.Equal(t, "11.22", api.bodies[0]["socket_id"])
	assert.NotContains(t, api.bodies[1], "socket_id", "without ToOthers everybody gets it")
}

func TestBroadcastToOthersWithoutSocketIDSendsToAll(t *testing.T) {
	api := newCapture(t)

	require.NoError(t, NewBroadcaster(api.publisher()).Broadcast(context.Background(), []string{"n"}, "E", nil, ToOthers()))

	assert.NotContains(t, api.bodies[0], "socket_id")
}

func TestBroadcastWithExplicitSocketID(t *testing.T) {
	api := newCapture(t)

	require.NoError(t, NewBroadcaster(api.publisher()).Broadcast(context.Background(), []string{"n"}, "E", nil, ExceptSocket("9.9")))

	assert.Equal(t, "9.9", api.bodies[0]["socket_id"])
}

func TestBroadcastWrapsPublisherError(t *testing.T) {
	api := newCapture(t)
	api.status = http.StatusInternalServerError

	err := NewBroadcaster(api.publisher()).Broadcast(context.Background(), []string{"n"}, "E", nil)

	assert.ErrorContains(t, err, "broadcast event E")
	assert.ErrorContains(t, err, "500")
}

func TestSocketIDMiddlewareStoresHeaderInContext(t *testing.T) {
	ctx := mockshttp.NewContext(t)
	req := mockshttp.NewContextRequest(t)
	req.EXPECT().Header("X-Socket-Id").Return("5.6")
	req.EXPECT().Next()
	ctx.EXPECT().Request().Return(req)
	base := context.Background()
	ctx.EXPECT().Context().Return(base)
	var stored context.Context
	ctx.EXPECT().WithContext(mock.Anything).Run(func(c context.Context) { stored = c })

	SocketIDMiddleware().Handle(ctx)

	assert.Equal(t, "5.6", SocketIDFrom(stored))
}

func TestSocketIDMiddlewareSignature(t *testing.T) {
	assert.Equal(t, "reverb.socket_id", SocketIDMiddleware().Signature())
}

func TestSocketIDMiddlewareIgnoresBadHeader(t *testing.T) {
	ctx := mockshttp.NewContext(t)
	req := mockshttp.NewContextRequest(t)
	req.EXPECT().Header("X-Socket-Id").Return("not a socket id")
	req.EXPECT().Next()
	ctx.EXPECT().Request().Return(req)

	SocketIDMiddleware().Handle(ctx)
}
