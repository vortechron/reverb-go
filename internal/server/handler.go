package server

import (
	"bytes"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"strings"

	"github.com/vortechron/reverb-go/broadcast"
	"github.com/vortechron/reverb-go/internal/hub"
	"github.com/vortechron/reverb-go/internal/wire"
)

var errInvalid = errors.New("invalid message format")

type incoming struct {
	Event   string          `json:"event"`
	Channel *string         `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

// handle processes one text frame. Every failure becomes a pusher:error frame.
func (s *session) handle(raw []byte) {
	s.touch()
	var m incoming
	if err := json.Unmarshal(raw, &m); err != nil || m.Event == "" {
		s.Send(wire.Error(wire.CodeInvalidMessage, "Invalid message format"))
		return
	}
	m.Data = normalizeData(m.Data)
	var err error
	if strings.HasPrefix(m.Event, "pusher:") {
		err = s.handlePusher(m)
	} else {
		err = s.handleClientEvent(m)
	}
	if err != nil {
		s.Send(wire.Error(wire.CodeInvalidMessage, "Invalid message format"))
	}
}

// normalizeData turns a JSON string that holds JSON into the JSON itself,
// which is what Reverb does before it looks at data.
func normalizeData(raw json.RawMessage) json.RawMessage {
	var inner string
	if len(raw) == 0 || json.Unmarshal(raw, &inner) != nil || !json.Valid([]byte(inner)) {
		return raw
	}
	return json.RawMessage(inner)
}

func (s *session) handlePusher(m incoming) error {
	switch strings.TrimPrefix(m.Event, "pusher:") {
	case "ping":
		s.Send(wire.Pong())
	case "pong":
		// touch() already ran
	case "subscribe":
		return s.subscribe(m.Data)
	case "unsubscribe":
		return s.unsubscribe(m.Data)
	default:
		return errInvalid
	}
	return nil
}

type subscribePayload struct {
	Channel     *string         `json:"channel"`
	Auth        json.RawMessage `json:"auth"`
	ChannelData json.RawMessage `json:"channel_data"`
}

func (s *session) subscribe(data json.RawMessage) error {
	name, auth, channelData, err := parseSubscribe(data)
	if err != nil {
		return err
	}
	if hub.KindOf(name).NeedsAuth() && !s.validAuth(auth, name, channelData) {
		s.Send(wire.Error(wire.CodeUnauthorized, "Connection is unauthorized"))
		return nil
	}
	if err := s.hub.Subscribe(s, name, channelData); err != nil {
		return errInvalid
	}
	return nil
}

func parseSubscribe(data json.RawMessage) (name, auth, channelData string, err error) {
	var p subscribePayload
	if err := json.Unmarshal(data, &p); err != nil || p.Channel == nil || !broadcast.ValidChannelName(*p.Channel) {
		return "", "", "", errInvalid
	}
	if auth, err = optionalString(p.Auth); err != nil {
		return "", "", "", err
	}
	if channelData, err = optionalString(p.ChannelData); err != nil {
		return "", "", "", err
	}
	if channelData != "" && !json.Valid([]byte(channelData)) {
		return "", "", "", errInvalid
	}
	return *p.Channel, auth, channelData, nil
}

func (s *session) unsubscribe(data json.RawMessage) error {
	var p subscribePayload
	if err := json.Unmarshal(data, &p); err != nil || p.Channel == nil {
		return errInvalid
	}
	s.hub.Unsubscribe(s, *p.Channel)
	return nil
}

// optionalString accepts null, nothing, or a JSON string.
func optionalString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	var v string
	if err := json.Unmarshal(raw, &v); err != nil {
		return "", errInvalid
	}
	return v, nil
}

// validAuth checks "key:signature" like Reverb: the key part is not compared,
// everything after the first colon must equal the HMAC.
func (s *session) validAuth(auth, channel, channelData string) bool {
	if !broadcast.ValidSocketID(s.id) {
		return false
	}
	signed := s.id + ":" + channel
	if channelData != "" {
		signed += ":" + channelData
	}
	want := broadcast.HMACHex(s.app.Secret, signed)
	got := auth
	if i := strings.Index(auth, ":"); i >= 0 {
		got = auth[i+1:]
	}
	return hmac.Equal([]byte(want), []byte(got))
}
