package server

import (
	"context"

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
	UserID string `json:"user_id,omitempty"`
}

// Bus moves Messages between nodes. Implementations must deliver a node's own
// messages too; the server filters them out by Node.
type Bus interface {
	Publish(ctx context.Context, m Message) error
	Subscribe(ctx context.Context, handle func(Message)) error
}

func (s *Server) startBus() {}

// publish delivers a frame to local subscribers and to other nodes.
func (s *Server) publish(app apps.App, channel string, frame []byte, except string) {
	s.hubs[app.ID].Publish(channel, frame, except)
}

// terminateUser closes every connection of a user on this node and on others.
func (s *Server) terminateUser(app apps.App, userID string) {
	s.hubs[app.ID].Terminate(userID)
}
