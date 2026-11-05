package server_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vortechron/reverb-go/broadcast"
	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/server"
)

type apiCall struct {
	method, path string
	query        url.Values
	body         string
	timestamp    time.Time
	tamper       func(url.Values)
}

func (h *harness) call(t *testing.T, c apiCall) (int, string) {
	t.Helper()
	if c.timestamp.IsZero() {
		c.timestamp = time.Now()
	}
	q := url.Values{}
	for k, v := range c.query {
		q[k] = v
	}
	q.Set("auth_key", testCreds.Key)
	q.Set("auth_timestamp", strconv.FormatInt(c.timestamp.Unix(), 10))
	q.Set("auth_version", "1.0")
	body := []byte(c.body)
	if len(body) > 0 {
		q.Set("body_md5", "ignored-by-server")
	}
	signPath := c.path
	q.Set("auth_signature", broadcast.RequestSignature(testCreds.Secret, c.method, signPath, q, body))
	if c.tamper != nil {
		c.tamper(q)
	}
	req, err := http.NewRequestWithContext(t.Context(), c.method, h.http.URL+c.path+"?"+q.Encode(), bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, strings.TrimSpace(string(out))
}

func (h *harness) post(t *testing.T, endpoint, body string) (int, string) {
	t.Helper()
	return h.call(t, apiCall{method: "POST", path: "/apps/app1/" + endpoint, body: body})
}

func (h *harness) get(t *testing.T, path string, query url.Values) (int, string) {
	t.Helper()
	return h.call(t, apiCall{method: "GET", path: "/apps/app1/" + path, query: query})
}

func TestEventsDeliverToSubscribersExceptSocket(t *testing.T) {
	h := newHarness(t)
	a, b := h.connect(t), h.connect(t)
	a.subscribeOK("news", "", "")
	b.subscribeOK("news", "", "")
	body := `{"name":"App\\Events\\Hello","channels":["news","other"],"data":"{\"x\":1}","socket_id":"` + a.SocketID + `"}`
	if code, out := h.post(t, "events", body); code != 200 || out != "{}" {
		t.Fatalf("status %d body %s", code, out)
	}
	f := b.read()
	if f.Event != `App\Events\Hello` || f.Channel != "news" || f.dataString() != `{"x":1}` {
		t.Fatalf("b got %+v", f)
	}
	a.expectSilence()
}

func TestEventsAcceptSingleChannelField(t *testing.T) {
	h := newHarness(t)
	a := h.connect(t)
	a.subscribeOK("news", "", "")
	if code, _ := h.post(t, "events", `{"name":"e","channel":"news","data":"d"}`); code != 200 {
		t.Fatalf("status %d", code)
	}
	if f := a.read(); f.Event != "e" || f.dataString() != "d" {
		t.Fatalf("got %+v", f)
	}
}

func TestEventsReturnChannelInfoWhenAsked(t *testing.T) {
	h := newHarness(t)
	h.connect(t).subscribeOK("news", "", "")
	code, out := h.post(t, "events", `{"name":"e","channels":["news","empty"],"data":"d","info":"subscription_count"}`)
	if code != 200 || out != `{"channels":{"empty":{},"news":{"subscription_count":1}}}` {
		t.Fatalf("status %d body %s", code, out)
	}
}

func TestAPIRejectsBadSignatures(t *testing.T) {
	h := newHarness(t)
	path := "/apps/app1/events"
	body := `{"name":"e","channel":"c","data":"d"}`
	cases := map[string]apiCall{
		"wrong signature": {method: "POST", path: path, body: body, tamper: func(q url.Values) { q.Set("auth_signature", "00") }},
		"no signature":    {method: "POST", path: path, body: body, tamper: func(q url.Values) { q.Del("auth_signature") }},
		"changed query":   {method: "POST", path: path, body: body, tamper: func(q url.Values) { q.Set("auth_version", "2.0") }},
		"old timestamp":   {method: "POST", path: path, body: body, timestamp: time.Now().Add(-11 * time.Minute)},
		"future time":     {method: "POST", path: path, body: body, timestamp: time.Now().Add(11 * time.Minute)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if code, _ := h.call(t, c); code != 401 {
				t.Fatalf("status = %d, want 401", code)
			}
		})
	}
}

func TestAPIRejectsTamperedBody(t *testing.T) {
	h := newHarness(t)
	req, _ := http.NewRequestWithContext(t.Context(), "POST", h.http.URL+"/apps/app1/events?auth_signature=x", strings.NewReader("{}"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestEventsValidateBody(t *testing.T) {
	h := newHarness(t)
	bad := map[string]string{
		"no name":     `{"channel":"c","data":"d"}`,
		"no data":     `{"name":"e","channel":"c"}`,
		"data object": `{"name":"e","channel":"c","data":{"a":1}}`,
		"no channels": `{"name":"e","data":"d"}`,
	}
	for name, body := range bad {
		if code, _ := h.post(t, "events", body); code != 422 {
			t.Errorf("%s: status = %d, want 422", name, code)
		}
	}
	if code, _ := h.post(t, "events", `{not json`); code != 400 {
		t.Errorf("broken json: status = %d, want 400", code)
	}
}

func TestAPIUnknownAppIs404(t *testing.T) {
	h := newHarness(t)
	if code, _ := h.call(t, apiCall{method: "POST", path: "/apps/other/events", body: `{}`}); code != 404 {
		t.Fatalf("status = %d", code)
	}
}

func TestAPIBodyTooLargeIs413(t *testing.T) {
	h := newHarnessOpts(t, server.Options{MaxRequestSize: 100})
	body := `{"name":"e","channel":"c","data":"` + strings.Repeat("a", 200) + `"}`
	if code, _ := h.post(t, "events", body); code != 413 {
		t.Fatalf("status = %d", code)
	}
}

func TestBatchEvents(t *testing.T) {
	h := newHarness(t)
	a := h.connect(t)
	a.subscribeOK("c1", "", "")
	a.subscribeOK("c2", "", "")
	body := `{"batch":[{"name":"e1","channel":"c1","data":"1"},{"name":"e2","channel":"c2","data":"2"}]}`
	if code, out := h.post(t, "batch_events", body); code != 200 || out != `{"batch":{}}` {
		t.Fatalf("status %d body %s", code, out)
	}
	if f := a.read(); f.Event != "e1" {
		t.Fatalf("got %+v", f)
	}
	if f := a.read(); f.Event != "e2" {
		t.Fatalf("got %+v", f)
	}
	if code, _ := h.post(t, "batch_events", `{"batch":[{"name":"e1","data":"1"}]}`); code != 422 {
		t.Fatalf("status = %d", code)
	}
}

func TestBatchEventsReturnInfo(t *testing.T) {
	h := newHarness(t)
	h.connect(t).subscribeOK("c1", "", "")
	body := `{"batch":[{"name":"e1","channel":"c1","data":"1","info":"subscription_count"},{"name":"e2","channel":"c2","data":"2"}]}`
	code, out := h.post(t, "batch_events", body)
	if code != 200 || out != `{"batch":[{"subscription_count":1},{}]}` {
		t.Fatalf("status %d body %s", code, out)
	}
}

func TestChannelEndpoints(t *testing.T) {
	h := newHarness(t)
	a, b := h.connect(t), h.connect(t)
	a.subscribeOK("news", "", "")
	auth, data := h.presence(t, b, 7, map[string]string{"n": "x"})
	b.subscribeOK("presence-r", auth, data)

	code, out := h.get(t, "channels", url.Values{"info": {"user_count,subscription_count"}})
	want := `{"channels":{"news":{"subscription_count":1},"presence-r":{"user_count":1}}}`
	if code != 200 || out != want {
		t.Fatalf("channels: %d %s", code, out)
	}
	code, out = h.get(t, "channels", url.Values{"filter_by_prefix": {"presence-"}})
	if code != 200 || out != `{"channels":{"presence-r":{}}}` {
		t.Fatalf("filtered: %d %s", code, out)
	}
	code, out = h.get(t, "channels/news", url.Values{"info": {"subscription_count"}})
	if code != 200 || out != `{"occupied":true,"subscription_count":1}` {
		t.Fatalf("channel: %d %s", code, out)
	}
	code, out = h.get(t, "channels/missing", nil)
	if code != 200 || out != `{"occupied":false}` {
		t.Fatalf("missing channel: %d %s", code, out)
	}
	code, out = h.get(t, "channels/presence-r/users", nil)
	if code != 200 || out != `{"users":[{"id":7}]}` {
		t.Fatalf("users: %d %s", code, out)
	}
	if code, _ := h.get(t, "channels/news/users", nil); code != 400 {
		t.Fatalf("users of public channel: %d", code)
	}
	if code, _ := h.get(t, "channels/missing/users", nil); code != 404 {
		t.Fatalf("users of missing channel: %d", code)
	}
	code, out = h.get(t, "connections", nil)
	if code != 200 || out != `{"connections":2}` {
		t.Fatalf("connections: %d %s", code, out)
	}
}

func TestChannelInfoShowsCache(t *testing.T) {
	h := newHarness(t)
	h.connect(t).subscribeOK("cache-x", "", "")
	h.post(t, "events", `{"name":"e","channel":"cache-x","data":"d"}`)
	code, out := h.get(t, "channels/cache-x", url.Values{"info": {"cache"}})
	if code != 200 || !strings.Contains(out, `"cache":{"event":"e"`) {
		t.Fatalf("%d %s", code, out)
	}
}

func TestTerminateConnections(t *testing.T) {
	h := newHarness(t)
	a := h.connect(t)
	auth, data := h.presence(t, a, 9, nil)
	a.subscribeOK("presence-r", auth, data)
	code, out := h.call(t, apiCall{method: "POST", path: "/apps/app1/users/9/terminate_connections"})
	if code != 200 || out != "{}" {
		t.Fatalf("%d %s", code, out)
	}
	if _, err := a.tryRead(2 * time.Second); err == nil {
		t.Fatal("connection should be closed")
	}
}

func TestHealthCheck(t *testing.T) {
	h := newHarness(t)
	resp, err := http.Get(h.http.URL + "/up") //nolint:noctx // plain test request
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || strings.TrimSpace(string(body)) != `{"health":"OK"}` {
		t.Fatalf("%d %s", resp.StatusCode, body)
	}
}

func TestPathPrefixIsStrippedBeforeSigning(t *testing.T) {
	h := newHarnessOpts(t, server.Options{Path: "/ws"})
	// signature covers the path without the prefix, like Reverb
	q := url.Values{"auth_key": {"key1"}, "auth_timestamp": {strconv.FormatInt(time.Now().Unix(), 10)}, "auth_version": {"1.0"}}
	q.Set("auth_signature", broadcast.RequestSignature(testCreds.Secret, "GET", "/apps/app1/connections", q, nil))
	req, _ := http.NewRequestWithContext(t.Context(), "GET", h.http.URL+"/ws/apps/app1/connections?"+q.Encode(), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d", resp.StatusCode)
	}
}

func TestSlowConsumerIsDisconnected(t *testing.T) {
	h := newHarnessOpts(t, server.Options{SendQueue: 4}, func(a *apps.App) { a.MaxMessageSize = 1 << 20 })
	slow := h.connect(t)
	slow.subscribeOK("news", "", "")
	big, _ := json.Marshal(map[string]any{"name": "e", "channel": "news", "data": strings.Repeat("x", 9000)})
	closed := false
	for range 3000 {
		h.post(t, "events", string(big))
		if h.srv.ConnectionCount(testCreds.AppID) == 0 {
			closed = true
			break
		}
	}
	if !closed {
		t.Fatal("slow consumer was never disconnected")
	}
}

func TestAPIChecksAuthKeyAgainstTheApp(t *testing.T) {
	h := newHarness(t)
	body := `{"name":"e","channel":"c","data":"d"}`
	code, _ := h.call(t, apiCall{method: "POST", path: "/apps/app1/events", body: body, tamper: func(q url.Values) {
		q.Set("auth_key", "someone-elses-key")
		q.Set("auth_signature", broadcast.RequestSignature(testCreds.Secret, "POST", "/apps/app1/events",
			withoutSignature(q), []byte(body)))
	}})
	if code != 401 {
		t.Fatalf("status = %d, want 401", code)
	}
}

func withoutSignature(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		if k != "auth_signature" && k != "body_md5" {
			out[k] = v
		}
	}
	return out
}

func TestAPILimitsOnEventsAndBatches(t *testing.T) {
	h := newHarness(t)
	many := make([]string, 101)
	for i := range many {
		many[i] = "c" + strconv.Itoa(i)
	}
	tooManyChannels, _ := json.Marshal(map[string]any{"name": "e", "channels": many, "data": "d"})
	longName, _ := json.Marshal(map[string]any{"name": strings.Repeat("n", 201), "channel": "c", "data": "d"})
	items := make([]map[string]string, 11)
	for i := range items {
		items[i] = map[string]string{"name": "e", "channel": "c", "data": "d"}
	}
	bigBatch, _ := json.Marshal(map[string]any{"batch": items})
	cases := map[string]struct{ endpoint, body string }{
		"101 channels": {"events", string(tooManyChannels)},
		"long name":    {"events", string(longName)},
		"11 events":    {"batch_events", string(bigBatch)},
	}
	for name, c := range cases {
		if code, _ := h.post(t, c.endpoint, c.body); code != 422 {
			t.Errorf("%s: status = %d, want 422", name, code)
		}
	}
	ten := items[:10]
	okBatch, _ := json.Marshal(map[string]any{"batch": ten})
	if code, _ := h.post(t, "batch_events", string(okBatch)); code != 200 {
		t.Errorf("10 events: status = %d, want 200", code)
	}
}

func TestAPIRequestsHaveAReadDeadline(t *testing.T) {
	h := newHarnessOpts(t, server.Options{APIReadTimeout: 200 * time.Millisecond})
	conn, err := (&net.Dialer{}).DialContext(t.Context(), "tcp", strings.TrimPrefix(h.http.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// Announce 50 bytes, send 1, then go quiet.
	_, _ = conn.Write([]byte("POST /apps/app1/events HTTP/1.1\r\nHost: x\r\nContent-Length: 50\r\n\r\n{"))
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || !strings.HasPrefix(line, "HTTP/1.1 408") {
		t.Fatalf("got %q err=%v, want a 408 response", line, err)
	}
}

func TestWebSocketsAreNotCutByTheAPIDeadline(t *testing.T) {
	h := newHarnessOpts(t, server.Options{APIReadTimeout: 100 * time.Millisecond})
	c := h.connect(t)
	time.Sleep(400 * time.Millisecond)
	c.send(map[string]string{"event": "pusher:ping"})
	if f := c.read(); f.Event != "pusher:pong" {
		t.Fatalf("frame = %+v", f)
	}
}
