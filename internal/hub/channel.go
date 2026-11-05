package hub

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
)

func (ch *channel) hasUser(userID string) bool {
	for _, s := range ch.subs {
		if s.member.UserID == userID {
			return true
		}
	}
	return false
}

func (ch *channel) broadcast(frame []byte, exceptID string) {
	for id, s := range ch.subs {
		if id != exceptID {
			s.conn.Send(frame)
		}
	}
}

func (ch *channel) replay(c Conn) {
	if ch.cached != nil {
		c.Send(ch.cached)
		return
	}
	c.Send(cacheMiss(ch.name))
}

// ordered returns the subscriptions in join order.
func (ch *channel) ordered() []*sub {
	list := make([]*sub, 0, len(ch.subs))
	for _, s := range ch.subs {
		list = append(list, s)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].seq < list[j].seq })
	return list
}

// uniqueUsers returns one subscription per user. When someone has no user_id
// it returns nothing, because Reverb then reports an empty list.
func (ch *channel) uniqueUsers() (users []*sub) {
	seen := map[string]bool{}
	for _, s := range ch.ordered() {
		if !s.member.HasUser() {
			return nil
		}
		if !seen[s.member.UserID] {
			seen[s.member.UserID] = true
			users = append(users, s)
		}
	}
	return users
}

func (ch *channel) successData() string {
	if !ch.kind.IsPresence() {
		return "{}"
	}
	users := ch.uniqueUsers()
	var ids, hash []string
	for _, u := range users {
		ids = append(ids, string(u.member.RawUserID))
		key, _ := json.Marshal(u.member.UserID)
		info := string(u.member.UserInfo)
		if info == "" {
			info = "null"
		}
		hash = append(hash, string(key)+":"+info)
	}
	return `{"presence":{"count":` + strconv.Itoa(len(users)) + `,"ids":[` + strings.Join(ids, ",") +
		`],"hash":{` + strings.Join(hash, ",") + `}}}`
}
