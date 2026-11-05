package server

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/hub"
	"github.com/vortechron/reverb-go/internal/wire"
)

// handleClientEvent re-sends a client-* event (a whisper) to the other
// subscribers of the channel. Rules follow Reverb's ClientEvent class.
func (s *session) handleClientEvent(m incoming) error {
	if m.Channel == nil || !validClientData(m.Data) {
		return errInvalid
	}
	if !strings.HasPrefix(m.Event, "client-") {
		return nil
	}
	channel := *m.Channel
	if s.app.ClientEvents != apps.ClientEventsMembers && s.app.ClientEvents != apps.ClientEventsAll {
		s.Send(wire.Error(wire.CodeRateLimit, "The app does not have client messaging enabled."))
		return nil
	}
	member, joined := s.hub.Member(s, channel)
	if !s.mayWhisper(channel, joined) {
		s.Send(wire.Error(wire.CodeUnauthorized, "The client is not a member of the specified channel."))
		return nil
	}
	var userID json.RawMessage
	if joined && member.HasUser() {
		userID = member.RawUserID
	}
	frame := hub.ClientEventFrame(m.Event, channel, nullToEmpty(m.Data), userID)
	s.srv.publish(s.app, channel, frame, s.id)
	return nil
}

// mayWhisper allows private and presence channels, and in "members" mode only joined ones.
func (s *session) mayWhisper(channel string, joined bool) bool {
	if !hub.KindOf(channel).NeedsAuth() {
		return false
	}
	return joined || s.app.ClientEvents == apps.ClientEventsAll
}

// validClientData allows no data, null, an object or an array.
func validClientData(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return true
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}

func nullToEmpty(raw json.RawMessage) json.RawMessage {
	if string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	return raw
}
