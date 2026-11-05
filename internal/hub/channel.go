package hub

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/vortechron/reverb-go/internal/wire"
)

type sub struct {
	conn   Conn
	member Member
}

// user is one presence user. A user can hold several sockets.
type user struct {
	rawID json.RawMessage
	info  json.RawMessage
	count int
}

type channel struct {
	name     string
	kind     Kind
	subs     map[string]*sub
	users    map[string]*user // presence only, keyed by user id
	snapshot []Conn           // immutable copy of the subscribers, nil when stale
	cached   []byte
}

func newChannel(name string) *channel {
	return &channel{name: name, kind: KindOf(name), subs: map[string]*sub{}, users: map[string]*user{}}
}

// conns returns the subscribers. The slice is never changed after it is built,
// so it can be used after the hub lock is released.
func (ch *channel) conns() []Conn {
	if ch.snapshot == nil {
		ch.snapshot = make([]Conn, 0, len(ch.subs))
		for _, s := range ch.subs {
			ch.snapshot = append(ch.snapshot, s.conn)
		}
	}
	return ch.snapshot
}

// join adds or replaces a subscription and returns the member events to send.
func (ch *channel) join(c Conn, m Member) []fanout {
	old, had := ch.subs[c.ID()]
	ch.subs[c.ID()] = &sub{conn: c, member: m}
	if !had {
		ch.snapshot = nil
	}
	if !ch.kind.IsPresence() {
		return nil
	}
	var out []fanout
	if had && old.member.UserID == m.UserID {
		ch.users[m.UserID].info = m.UserInfo
		return nil
	}
	if had {
		out = append(out, ch.dropUser(old.member, c.ID())...)
	}
	if ch.addUser(m) {
		out = append(out, fanout{targets: ch.conns(), frame: wire.Internal("member_added", ch.name, m.Raw), except: c.ID()})
	}
	return out
}

// leave removes a subscription and returns the member_removed event, if any.
func (ch *channel) leave(c Conn) []fanout {
	s, ok := ch.subs[c.ID()]
	if !ok {
		return nil
	}
	delete(ch.subs, c.ID())
	ch.snapshot = nil
	if !ch.kind.IsPresence() {
		return nil
	}
	return ch.dropUser(s.member, c.ID())
}

// addUser counts one more socket for the user. It reports whether the user is new.
func (ch *channel) addUser(m Member) bool {
	u, ok := ch.users[m.UserID]
	if !ok {
		ch.users[m.UserID] = &user{rawID: m.RawUserID, info: m.UserInfo, count: 1}
		return true
	}
	u.count++
	return false
}

// dropUser counts one socket less and returns member_removed when it was the last.
func (ch *channel) dropUser(m Member, exceptID string) []fanout {
	u, ok := ch.users[m.UserID]
	if !ok {
		return nil
	}
	if u.count--; u.count > 0 {
		return nil
	}
	delete(ch.users, m.UserID)
	frame := wire.Internal("member_removed", ch.name, `{"user_id":`+string(m.RawUserID)+`}`)
	return []fanout{{targets: ch.conns(), frame: frame, except: exceptID}}
}

func (ch *channel) replay(c Conn) {
	if ch.cached != nil {
		c.Send(ch.cached)
		return
	}
	c.Send(wire.CacheMiss(ch.name))
}

// successData is the data of subscription_succeeded.
func (ch *channel) successData() string {
	if !ch.kind.IsPresence() {
		return "{}"
	}
	ids := make([]string, 0, len(ch.users))
	hash := make([]string, 0, len(ch.users))
	for key, u := range ch.users {
		ids = append(ids, string(u.rawID))
		quoted, _ := json.Marshal(key)
		info := string(u.info)
		if info == "" {
			info = "null"
		}
		hash = append(hash, string(quoted)+":"+info)
	}
	return `{"presence":{"count":` + strconv.Itoa(len(ch.users)) + `,"ids":[` + strings.Join(ids, ",") +
		`],"hash":{` + strings.Join(hash, ",") + `}}}`
}
