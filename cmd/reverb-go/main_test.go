package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/vortechron/reverb-go/internal/config"
)

func testEnv(extra map[string]string) func(string) string {
	m := map[string]string{"REVERB_APP_ID": "1", "REVERB_APP_KEY": "k", "REVERB_APP_SECRET": "s"}
	for k, v := range extra {
		m[k] = v
	}
	return func(k string) string { return m[k] }
}

func quiet() *slog.Logger { return slog.New(slog.DiscardHandler) }

func runUntilCancelled(t *testing.T, env func(string) string) (url string, stop func() error) {
	t.Helper()
	cfg, err := config.Load(env)
	if err != nil {
		t.Fatal(err)
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, cfg, ln, quiet()) }()
	return "http://" + ln.Addr().String(), func() error { cancel(); return <-done }
}

// waitHealthy polls /up until it answers 200. A node with scaling on answers 503
// until its Redis subscription is up.
func waitHealthy(t *testing.T, url string) (int, string) {
	t.Helper()
	var code int
	var body string
	for range 150 {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, url+"/up", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			code, body = resp.StatusCode, string(b)
			if code == http.StatusOK {
				return code, body
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	if code == 0 {
		t.Fatal("server did not start")
	}
	return code, body
}

func TestServeAnswersHealthCheckAndStopsOnCancel(t *testing.T) {
	url, stop := runUntilCancelled(t, testEnv(nil))
	if code, body := waitHealthy(t, url); code != 200 || body != "{\"health\":\"OK\"}\n" {
		t.Fatalf("%d %q", code, body)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

func TestServeWithRedisScaling(t *testing.T) {
	mr := miniredis.RunT(t)
	url, stop := runUntilCancelled(t, testEnv(map[string]string{
		"REVERB_SCALING_ENABLED": "true", "REVERB_SCALING_SECRET": "bus-secret", "REDIS_HOST": mr.Host(), "REDIS_PORT": mr.Port(),
	}))
	if code, _ := waitHealthy(t, url); code != 200 {
		t.Fatalf("status %d", code)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

func TestServeFailsWhenRedisIsUnreachable(t *testing.T) {
	cfg, err := config.Load(testEnv(map[string]string{
		"REVERB_SCALING_ENABLED": "true", "REVERB_SCALING_SECRET": "bus-secret", "REDIS_HOST": "127.0.0.1", "REDIS_PORT": "1",
	}))
	if err != nil {
		t.Fatal(err)
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if err := serve(t.Context(), cfg, ln, quiet()); err == nil {
		t.Fatal("expected an error")
	}
}

func TestStartRejectsBadConfigAndBusyPort(t *testing.T) {
	if err := start(t.Context(), func(string) string { return "" }, quiet()); err == nil {
		t.Fatal("expected config error")
	}
	lc := net.ListenConfig{}
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	env := testEnv(map[string]string{"REVERB_SERVER_HOST": "127.0.0.1", "REVERB_SERVER_PORT": port})
	if err := start(t.Context(), env, quiet()); err == nil {
		t.Fatal("expected listen error")
	}
}
