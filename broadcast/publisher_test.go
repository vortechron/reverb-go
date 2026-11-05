package broadcast_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vortechron/reverb-go/broadcast"
)

func TestRequestSignatureMatchesPusherDocs(t *testing.T) {
	params := url.Values{}
	params.Set("auth_key", "278d425bdf160c739803")
	params.Set("auth_timestamp", "1353088179")
	params.Set("auth_version", "1.0")
	body := []byte(`{"name":"foo","channels":["project-3"],"data":"{\"some\":\"data\"}"}`)
	got := broadcast.RequestSignature("7ad3773142a6692b25b8", http.MethodPost, "/apps/3/events", params, body)
	want := "da454824c97ba181a32ccc17a72625ba02771f50b50e1e7430e47a1f3f457e6c"
	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

type captured struct {
	method, path string
	query        url.Values
	body         []byte
}

func fakeServer(t *testing.T, status int) (*httptest.Server, func() []captured) {
	t.Helper()
	var mu sync.Mutex
	var got []captured
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, captured{r.Method, r.URL.Path, r.URL.Query(), b})
		mu.Unlock()
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []captured { mu.Lock(); defer mu.Unlock(); return append([]captured(nil), got...) }
}

func newPublisher(url string) *broadcast.Publisher {
	return broadcast.NewPublisher(broadcast.Config{
		Credentials: creds, BaseURL: url,
		Now: func() time.Time { return time.Unix(1353088179, 0) },
	})
}

func TestTriggerSendsSignedEvent(t *testing.T) {
	srv, calls := fakeServer(t, 200)
	err := newPublisher(srv.URL).Trigger(context.Background(), []string{"private-a", "b"},
		`App\Events\X`, map[string]int{"n": 1}, broadcast.WithSocketID("1.2"))
	if err != nil {
		t.Fatal(err)
	}
	c := calls()
	if len(c) != 1 || c[0].method != "POST" || c[0].path != "/apps/3/events" {
		t.Fatalf("calls = %+v", c)
	}
	var body struct {
		Name     string   `json:"name"`
		Data     string   `json:"data"`
		Channels []string `json:"channels"`
		SocketID string   `json:"socket_id"`
	}
	if err := json.Unmarshal(c[0].body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Name != `App\Events\X` || body.Data != `{"n":1}` || body.SocketID != "1.2" ||
		strings.Join(body.Channels, ",") != "private-a,b" {
		t.Fatalf("body = %+v", body)
	}
	q := c[0].query
	want := broadcast.RequestSignature(creds.Secret, "POST", c[0].path, withoutSig(q), c[0].body)
	if q.Get("auth_signature") != want || q.Get("auth_key") != creds.Key || q.Get("auth_version") != "1.0" {
		t.Fatalf("bad signed query: %v", q)
	}
}

func withoutSig(q url.Values) url.Values {
	out := url.Values{}
	for k, v := range q {
		if k != "auth_signature" {
			out[k] = v
		}
	}
	return out
}

func TestTriggerSplitsMoreThan100Channels(t *testing.T) {
	srv, calls := fakeServer(t, 200)
	chans := make([]string, 250)
	for i := range chans {
		chans[i] = "c" + strings.Repeat("x", i%3)
	}
	if err := newPublisher(srv.URL).Trigger(context.Background(), chans, "e", "raw-string"); err != nil {
		t.Fatal(err)
	}
	if n := len(calls()); n != 3 {
		t.Fatalf("expected 3 requests, got %d", n)
	}
	var body struct{ Data string }
	_ = json.Unmarshal(calls()[0].body, &body)
	if body.Data != "raw-string" {
		t.Fatalf("string data must be sent unchanged, got %q", body.Data)
	}
}

func TestTriggerBatchSendsOneRequest(t *testing.T) {
	srv, calls := fakeServer(t, 200)
	err := newPublisher(srv.URL).TriggerBatch(context.Background(), []broadcast.Event{
		{Channel: "a", Name: "e1", Data: 1}, {Channel: "b", Name: "e2", Data: "x", SocketID: "1.2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := calls()
	if len(c) != 1 || c[0].path != "/apps/3/batch_events" {
		t.Fatalf("calls = %+v", c)
	}
	var body struct {
		Batch []map[string]string `json:"batch"`
	}
	if err := json.Unmarshal(c[0].body, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Batch) != 2 || body.Batch[0]["data"] != "1" || body.Batch[1]["socket_id"] != "1.2" {
		t.Fatalf("body = %s", c[0].body)
	}
}

func TestTriggerReportsServerErrors(t *testing.T) {
	srv, _ := fakeServer(t, 401)
	err := newPublisher(srv.URL).Trigger(context.Background(), []string{"a"}, "e", "d")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

func TestTriggerValidatesInput(t *testing.T) {
	p := newPublisher("http://127.0.0.1:1")
	if err := p.Trigger(context.Background(), nil, "e", "d"); err == nil {
		t.Fatal("expected error for no channels")
	}
	if err := p.Trigger(context.Background(), []string{"a"}, "", "d"); err == nil {
		t.Fatal("expected error for empty name")
	}
	if err := p.Trigger(context.Background(), []string{"a"}, "e", make(chan int)); err == nil {
		t.Fatal("expected error for unencodable data")
	}
}
