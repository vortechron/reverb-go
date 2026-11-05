package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/vortechron/reverb-go/internal/hub"
)

func asSyntax(err error, target **json.SyntaxError) bool { return errors.As(err, target) }

// channelInfo builds the attribute map for one channel, following Reverb's
// InteractsWithChannelInformation: only the attributes named in flags appear.
func channelInfo(h *hub.Hub, name, flags string) map[string]any {
	want := map[string]bool{}
	for _, f := range strings.Split(flags, ",") {
		want[strings.TrimSpace(f)] = true
	}
	info, ok := h.Channel(name)
	if !ok {
		if want["occupied"] {
			return map[string]any{"occupied": false}
		}
		return map[string]any{}
	}
	return occupiedAttrs(info, want)
}

func occupiedAttrs(info hub.Info, want map[string]bool) map[string]any {
	out := map[string]any{}
	if want["occupied"] {
		out["occupied"] = info.SubscriptionCount > 0
	}
	if want["user_count"] && info.Kind.IsPresence() {
		out["user_count"] = info.UserCount
	}
	if want["subscription_count"] && !info.Kind.IsPresence() {
		out["subscription_count"] = info.SubscriptionCount
	}
	if want["cache"] && info.Kind.IsCache() && info.Cache != nil {
		out["cache"] = json.RawMessage(info.Cache)
	}
	return out
}

func (s *Server) channels(c *call) (int, any) {
	flags := c.query.Get("info")
	out := map[string]map[string]any{}
	for _, info := range c.hub.Channels(c.query.Get("filter_by_prefix")) {
		out[info.Name] = channelInfo(c.hub, info.Name, flags)
	}
	return http.StatusOK, map[string]any{"channels": out}
}

func (s *Server) channel(c *call) (int, any) {
	flags := "occupied"
	if extra := c.query.Get("info"); extra != "" {
		flags = extra + ",occupied"
	}
	return http.StatusOK, channelInfo(c.hub, c.r.PathValue("channel"), flags)
}

func (s *Server) channelUsers(c *call) (int, any) {
	ids, kind, found := c.hub.Users(c.r.PathValue("channel"))
	if !found {
		return http.StatusNotFound, struct{}{}
	}
	if !kind.IsPresence() {
		return http.StatusBadRequest, struct{}{}
	}
	users := make([]map[string]json.RawMessage, 0, len(ids))
	for _, id := range ids {
		users = append(users, map[string]json.RawMessage{"id": id})
	}
	return http.StatusOK, map[string]any{"users": users}
}

func (s *Server) connections(c *call) (int, any) {
	return http.StatusOK, map[string]int{"connections": c.hub.ConnectionCount()}
}
