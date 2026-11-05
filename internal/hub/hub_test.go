package hub_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vortechron/reverb-go/internal/hub"
	"github.com/vortechron/reverb-go/internal/wire"
)

type fakeConn struct {
	id     string
	mu     sync.Mutex
	got    []string
	closed int
}

func newConn(id string) *fakeConn { return &fakeConn{id: id} }

func (f *fakeConn) ID() string { return f.id }
func (f *fakeConn) Send(b []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.got = append(f.got, string(b))
}

func (f *fakeConn) Close(code int, _ string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = code
}

func (f *fakeConn) frames() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.got...)
}

func (f *fakeConn) last() string {
	g := f.frames()
	if len(g) == 0 {
		return ""
	}
	return g[len(g)-1]
}

func mustSub(t *testing.T, h *hub.Hub, c *fakeConn, ch, data string) {
	t.Helper()
	h.TryAdd(c, 0)
	if err := h.Subscribe(c, ch, data, hub.Limits{}); err != nil {
		t.Fatal(err)
	}
}

func TestPublicChannelDeliversExceptSender(t *testing.T) {
	h := hub.New()
	a, b := newConn("1.1"), newConn("2.2")
	mustSub(t, h, a, "news", "")
	mustSub(t, h, b, "news", "")
	if a.frames()[0] != `{"event":"pusher_internal:subscription_succeeded","data":"{}","channel":"news"}` {
		t.Fatalf("first frame = %s", a.frames()[0])
	}
	h.Publish("news", wire.Event("e", "news", "{}"), "1.1")
	if len(a.frames()) != 1 {
		t.Fatal("excluded socket must not get the event")
	}
	if !strings.Contains(b.last(), `"event":"e"`) {
		t.Fatalf("b got %v", b.frames())
	}
}

func TestUnsubscribeStopsDelivery(t *testing.T) {
	h := hub.New()
	a := newConn("1.1")
	mustSub(t, h, a, "news", "")
	h.Unsubscribe(a, "news")
	h.Publish("news", wire.Event("e", "news", "{}"), "")
	if len(a.frames()) != 1 {
		t.Fatalf("frames = %v", a.frames())
	}
	if _, ok := h.Channel("news"); ok {
		t.Fatal("empty channel must be removed")
	}
}

const alice = `{"user_id":"1","user_info":{"name":"Alice"}}`

func TestPresenceJoinAndLeaveEvents(t *testing.T) {
	h := hub.New()
	a, b := newConn("1.1"), newConn("2.2")
	mustSub(t, h, a, "presence-room", alice)
	if want := `"data":"{\"presence\":{\"count\":1,\"ids\":[\"1\"],\"hash\":{\"1\":{\"name\":\"Alice\"}}}}"`; !strings.Contains(a.last(), want) {
		t.Fatalf("succeeded = %s", a.last())
	}
	mustSub(t, h, b, "presence-room", `{"user_id":2,"user_info":{"name":"Bob"}}`)
	if !strings.Contains(a.last(), `pusher_internal:member_added`) || !strings.Contains(a.last(), `\"user_id\":2`) {
		t.Fatalf("a should see member_added, got %s", a.last())
	}
	if strings.Contains(b.last(), "member_added") {
		t.Fatal("joiner must not get its own member_added")
	}
	h.Remove(b)
	if want := `{"event":"pusher_internal:member_removed","data":"{\"user_id\":2}","channel":"presence-room"}`; a.last() != want {
		t.Fatalf("a got %s", a.last())
	}
}

func TestPresenceSameUserTwoSocketsEmitsOnce(t *testing.T) {
	h := hub.New()
	a, b1, b2 := newConn("1.1"), newConn("2.2"), newConn("3.3")
	mustSub(t, h, a, "presence-r", alice)
	mustSub(t, h, b1, "presence-r", `{"user_id":"7"}`)
	mustSub(t, h, b2, "presence-r", `{"user_id":"7"}`)
	added := 0
	for _, f := range a.frames() {
		if strings.Contains(f, "member_added") {
			added++
		}
	}
	if added != 1 {
		t.Fatalf("member_added count = %d", added)
	}
	h.Remove(b1)
	if strings.Contains(a.last(), "member_removed") {
		t.Fatal("user still has another socket, no member_removed yet")
	}
	h.Remove(b2)
	if !strings.Contains(a.last(), "member_removed") {
		t.Fatalf("last socket left, want member_removed, got %s", a.last())
	}
	info, _ := h.Channel("presence-r")
	if info.UserCount != 1 {
		t.Fatalf("user count = %d", info.UserCount)
	}
}

func TestPresenceWithoutUserIDIsRejected(t *testing.T) {
	h := hub.New()
	a := newConn("1.1")
	h.TryAdd(a, 0)
	for _, data := range []string{"", `{"user_info":{}}`, `{"user_id":null}`} {
		if err := h.Subscribe(a, "presence-r", data, hub.Limits{}); !errors.Is(err, hub.ErrMemberRequired) {
			t.Fatalf("channel_data %q: err = %v", data, err)
		}
	}
	if _, ok := h.Channel("presence-r"); ok {
		t.Fatal("a rejected subscribe must not leave a channel behind")
	}
}

func TestSubscribeRejectsBadChannelData(t *testing.T) {
	h := hub.New()
	a := newConn("1.1")
	h.TryAdd(a, 0)
	if err := h.Subscribe(a, "presence-r", "{not json", hub.Limits{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestCacheChannelReplaysLastEvent(t *testing.T) {
	h := hub.New()
	a, b := newConn("1.1"), newConn("2.2")
	mustSub(t, h, a, "cache-stock", "")
	if a.last() != `{"event":"pusher:cache_miss","channel":"cache-stock"}` {
		t.Fatalf("want cache_miss, got %s", a.last())
	}
	frame := wire.Event("price", "cache-stock", `{"p":1}`)
	h.Publish("cache-stock", frame, "")
	mustSub(t, h, b, "cache-stock", "")
	if b.last() != string(frame) {
		t.Fatalf("late joiner must get cached frame, got %s", b.last())
	}
	info, _ := h.Channel("cache-stock")
	if string(info.Cache) != string(frame) {
		t.Fatal("info must expose the cache")
	}
}

func TestMemberLookup(t *testing.T) {
	h := hub.New()
	a := newConn("1.1")
	mustSub(t, h, a, "presence-r", alice)
	m, ok := h.Member(a, "presence-r")
	if !ok || m.UserID != "1" {
		t.Fatalf("member = %+v %v", m, ok)
	}
	if _, ok := h.Member(a, "presence-other"); ok {
		t.Fatal("not subscribed")
	}
}

func TestIntrospection(t *testing.T) {
	h := hub.New()
	a, b := newConn("1.1"), newConn("2.2")
	mustSub(t, h, a, "presence-r", alice)
	mustSub(t, h, b, "news", "")
	if h.ConnectionCount() != 2 {
		t.Fatalf("connections = %d", h.ConnectionCount())
	}
	if got := h.Channels("presence-"); len(got) != 1 || got[0].Name != "presence-r" || got[0].UserCount != 1 {
		t.Fatalf("channels = %+v", got)
	}
	if got := h.Channels(""); len(got) != 2 {
		t.Fatalf("channels = %+v", got)
	}
	users, kind, ok := h.Users("presence-r")
	if !ok || !kind.IsPresence() || len(users) != 1 || string(users[0]) != `"1"` {
		t.Fatalf("users = %v %v %v", users, kind, ok)
	}
	if _, _, ok := h.Users("nope"); ok {
		t.Fatal("unknown channel")
	}
	if _, ok := h.Channel("nope"); ok {
		t.Fatal("unknown channel")
	}
}

func TestTerminateClosesUserConnections(t *testing.T) {
	h := hub.New()
	a, b := newConn("1.1"), newConn("2.2")
	mustSub(t, h, a, "presence-r", alice)
	mustSub(t, h, b, "presence-r", `{"user_id":"2"}`)
	h.Terminate("1")
	if a.closed == 0 || b.closed != 0 {
		t.Fatalf("closed a=%d b=%d", a.closed, b.closed)
	}
}

func TestClientEventFrameCarriesUserID(t *testing.T) {
	frame := hub.ClientEventFrame("client-x", "presence-r", json.RawMessage(`{"a":1}`), json.RawMessage(`7`))
	if string(frame) != `{"event":"client-x","channel":"presence-r","data":{"a":1},"user_id":7}` {
		t.Fatalf("frame = %s", frame)
	}
	frame = hub.ClientEventFrame("client-x", "private-r", nil, nil)
	if string(frame) != `{"event":"client-x","channel":"private-r"}` {
		t.Fatalf("frame = %s", frame)
	}
}

func TestKindOfChannel(t *testing.T) {
	cases := map[string]struct{ presence, private, cache bool }{
		"news": {}, "private-a": {private: true}, "presence-a": {private: true, presence: true},
		// Reverb matches the prefix without the dash, so these need auth too.
		"privatefoo": {private: true}, "presencefoo": {private: true, presence: true}, "cachefoo": {cache: true},
		"cache-a": {cache: true}, "private-cache-a": {private: true, cache: true},
		"presence-cache-a": {private: true, presence: true, cache: true},
	}
	for name, want := range cases {
		k := hub.KindOf(name)
		if k.IsPresence() != want.presence || k.NeedsAuth() != want.private || k.IsCache() != want.cache {
			t.Errorf("%s: %v", name, k)
		}
	}
}

func TestTerminateEmptyUserClosesNobody(t *testing.T) {
	h := hub.New()
	a := newConn("1.1")
	mustSub(t, h, a, "news", "")
	h.Terminate("")
	if a.closed != 0 {
		t.Fatal("anonymous connections must survive an empty user id")
	}
}

func TestTryAddNeverExceedsLimitUnderConcurrency(t *testing.T) {
	h := hub.New()
	const limit, tries = 10, 200
	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := range tries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if h.TryAdd(newConn(fmt.Sprintf("%d.1", i)), limit) {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if accepted != limit || h.ConnectionCount() != limit {
		t.Fatalf("accepted %d, count %d, want %d", accepted, h.ConnectionCount(), limit)
	}
}

func TestTryAddWithZeroLimitIsUnlimited(t *testing.T) {
	h := hub.New()
	for i := range 50 {
		if !h.TryAdd(newConn(fmt.Sprintf("%d.2", i)), 0) {
			t.Fatal("limit 0 means unlimited")
		}
	}
}

func TestPublishNoCacheKeepsServerEventCached(t *testing.T) {
	h := hub.New()
	a, late := newConn("1.1"), newConn("2.2")
	mustSub(t, h, a, "private-cache-x", "")
	server := wire.Event("price", "private-cache-x", `{"p":1}`)
	h.Publish("private-cache-x", server, "")
	h.PublishNoCache("private-cache-x", []byte(`{"event":"client-x"}`), "1.1")
	mustSub(t, h, late, "private-cache-x", "")
	if late.last() != string(server) {
		t.Fatalf("late joiner got %s, want the server event", late.last())
	}
}

func TestResubscribeWithOtherUserRemovesTheOldUser(t *testing.T) {
	h := hub.New()
	watcher, mover := newConn("1.1"), newConn("2.2")
	mustSub(t, h, watcher, "presence-r", alice)
	mustSub(t, h, mover, "presence-r", `{"user_id":"7"}`)
	watcher.got = nil
	if err := h.Subscribe(mover, "presence-r", `{"user_id":"8"}`, hub.Limits{}); err != nil {
		t.Fatal(err)
	}
	frames := strings.Join(watcher.frames(), "\n")
	if !strings.Contains(frames, `member_removed","data":"{\"user_id\":\"7\"}`) ||
		!strings.Contains(frames, `member_added`) {
		t.Fatalf("watcher saw:\n%s", frames)
	}
	if info, _ := h.Channel("presence-r"); info.UserCount != 2 {
		t.Fatalf("user count = %d, want 2", info.UserCount)
	}
}

func TestResubscribeWithSameUserEmitsNothing(t *testing.T) {
	h := hub.New()
	watcher, other := newConn("1.1"), newConn("2.2")
	mustSub(t, h, watcher, "presence-r", alice)
	mustSub(t, h, other, "presence-r", `{"user_id":"7"}`)
	watcher.got = nil
	if err := h.Subscribe(other, "presence-r", `{"user_id":"7","user_info":{"n":1}}`, hub.Limits{}); err != nil {
		t.Fatal(err)
	}
	if len(watcher.frames()) != 0 {
		t.Fatalf("watcher saw %v", watcher.frames())
	}
}

func TestResubscribeKeepsUserWhenAnotherSocketHoldsIt(t *testing.T) {
	h := hub.New()
	watcher, a, b := newConn("1.1"), newConn("2.2"), newConn("3.3")
	mustSub(t, h, watcher, "presence-r", alice)
	mustSub(t, h, a, "presence-r", `{"user_id":"7"}`)
	mustSub(t, h, b, "presence-r", `{"user_id":"7"}`)
	watcher.got = nil
	if err := h.Subscribe(a, "presence-r", `{"user_id":"9"}`, hub.Limits{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(watcher.frames(), ""), "member_removed") {
		t.Fatal("user 7 is still connected through another socket")
	}
}

func TestSubscribeEnforcesLimits(t *testing.T) {
	h := hub.New()
	a, b, c := newConn("1.1"), newConn("2.2"), newConn("3.3")
	lim := hub.Limits{MaxChannels: 2, MaxMembers: 1}
	h.TryAdd(a, 0)
	h.TryAdd(b, 0)
	h.TryAdd(c, 0)
	for _, ch := range []string{"one", "two"} {
		if err := h.Subscribe(a, ch, "", lim); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.Subscribe(a, "two", "", lim); err != nil {
		t.Fatalf("re-subscribing to a joined channel is not a new channel: %v", err)
	}
	if err := h.Subscribe(a, "three", "", lim); !errors.Is(err, hub.ErrTooManyChannels) {
		t.Fatalf("err = %v", err)
	}
	if _, ok := h.Channel("three"); ok {
		t.Fatal("rejected channel must not exist")
	}
	if err := h.Subscribe(b, "presence-r", alice, lim); err != nil {
		t.Fatal(err)
	}
	if err := h.Subscribe(c, "presence-r", `{"user_id":"2"}`, lim); !errors.Is(err, hub.ErrChannelFull) {
		t.Fatalf("err = %v", err)
	}
	// the same user on another socket is not a new member
	if err := h.Subscribe(c, "presence-r", alice, lim); err != nil {
		t.Fatalf("same user must be allowed: %v", err)
	}
}

type blockingConn struct {
	*fakeConn
	release chan struct{}
	entered chan struct{}
}

func (b *blockingConn) Send(frame []byte) {
	select {
	case b.entered <- struct{}{}:
	default:
	}
	<-b.release
	b.fakeConn.Send(frame)
}

func TestPublishDoesNotHoldTheHubLockWhileSending(t *testing.T) {
	h := hub.New()
	slow := &blockingConn{fakeConn: newConn("1.1"), release: make(chan struct{}), entered: make(chan struct{}, 1)}
	h.TryAdd(slow, 0)
	close(slow.release) // let the subscribe frames through
	if err := h.Subscribe(slow, "news", "", hub.Limits{}); err != nil {
		t.Fatal(err)
	}
	slow.release = make(chan struct{})
	<-slow.entered // drain the first signal, if any
	go h.Publish("news", wire.Event("e", "news", "{}"), "")
	select {
	case <-slow.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("publish never reached the connection")
	}
	done := make(chan struct{})
	go func() { _ = h.ConnectionCount(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the hub lock was held while sending to a slow connection")
	}
	close(slow.release)
}
