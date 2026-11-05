package server

import (
	"context"

	"github.com/vortechron/reverb-go/broadcast"

	"github.com/vortechron/reverb-go/internal/apps"
)

// Message is what nodes send each other.
type Message struct {
	Node   string `json:"node"`
	Kind   string `json:"kind"` // "event" or "terminate"
	AppID  string `json:"app_id"`
	Name   string `json:"channel,omitempty"`
	Frame  []byte `json:"frame,omitempty"`
	Except string `json:"except,omitempty"`
	// NoCache marks client events, which must not replace a cache channel's cached event.
	NoCache bool   `json:"no_cache,omitempty"`
	UserID  string `json:"user_id,omitempty"`
}

// Bus moves Messages between nodes. Implementations must deliver a node's own
// messages too; the server filters them out by Node.
type Bus interface {
	Publish(ctx context.Context, m Message) error
	Subscribe(ctx context.Context, handle func(Message)) error
}

const (
	kindEvent     = "event"
	kindTerminate = "terminate"
)

// startBus listens for messages from other nodes.
func (s *Server) startBus() {
	if s.opts.Bus == nil {
		return
	}
	err := s.opts.Bus.Subscribe(s.ctx, s.fromBus)
	if err != nil {
		s.log.Error("bus subscribe failed", "error", err)
	}
}

// Healther is implemented by buses that can report a broken connection.
// /up answers 503 while it returns an error.
type Healther interface {
	Healthy() error
}

// validMessage checks a message from another node before it touches a hub.
// The signature proves who sent it; this proves it makes sense.
func (s *Server) validMessage(m Message) bool {
	if m.Node == s.nodeID {
		return false
	}
	if _, ok := s.hubs[m.AppID]; !ok {
		return false
	}
	switch m.Kind {
	case kindEvent:
		return broadcast.ValidChannelName(m.Name) && len(m.Frame) > 0 &&
			(m.Except == "" || broadcast.ValidSocketID(m.Except))
	case kindTerminate:
		return m.UserID != ""
	}
	return false
}

func (s *Server) fromBus(m Message) {
	if !s.validMessage(m) {
		s.log.Warn("ignoring invalid bus message", "kind", m.Kind, "app_id", m.AppID)
		return
	}
	h := s.hubs[m.AppID]
	switch m.Kind {
	case kindEvent:
		if m.NoCache {
			h.PublishNoCache(m.Name, m.Frame, m.Except)
			return
		}
		h.Publish(m.Name, m.Frame, m.Except)
	case kindTerminate:
		h.Terminate(m.UserID)
	}
}

func (s *Server) toBus(m Message) {
	if s.opts.Bus == nil {
		return
	}
	m.Node = s.nodeID
	if err := s.opts.Bus.Publish(s.ctx, m); err != nil {
		s.log.Error("bus publish failed", "error", err)
	}
}

// publish delivers a frame to local subscribers and to other nodes.
func (s *Server) publish(app apps.App, channel string, frame []byte, except string) {
	s.hubs[app.ID].Publish(channel, frame, except)
	s.toBus(Message{Kind: kindEvent, AppID: app.ID, Name: channel, Frame: frame, Except: except})
}

// publishClientEvent delivers a whisper. It is never cached.
func (s *Server) publishClientEvent(app apps.App, channel string, frame []byte, except string) {
	s.hubs[app.ID].PublishNoCache(channel, frame, except)
	s.toBus(Message{Kind: kindEvent, AppID: app.ID, Name: channel, Frame: frame, Except: except, NoCache: true})
}

// terminateUser closes every connection of a user on this node and on others.
func (s *Server) terminateUser(app apps.App, userID string) {
	s.hubs[app.ID].Terminate(userID)
	s.toBus(Message{Kind: kindTerminate, AppID: app.ID, UserID: userID})
}
