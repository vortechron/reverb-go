package hub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Member is what a subscriber told us about itself in channel_data.
type Member struct {
	UserID    string          // string form, used to compare users
	RawUserID json.RawMessage // as sent, so numbers stay numbers
	UserInfo  json.RawMessage
	Raw       string // compact channel_data
}

// HasUser is true when channel_data carried a user_id.
func (m Member) HasUser() bool { return m.UserID != "" }

// ParseMember reads a channel_data string. An empty string gives an empty Member.
func ParseMember(channelData string) (Member, error) {
	if channelData == "" {
		return Member{}, nil
	}
	var doc struct {
		UserID   json.RawMessage `json:"user_id"`
		UserInfo json.RawMessage `json:"user_info"`
	}
	if err := json.Unmarshal([]byte(channelData), &doc); err != nil {
		return Member{}, fmt.Errorf("hub: channel_data is not a JSON object: %w", err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(channelData)); err != nil {
		return Member{}, fmt.Errorf("hub: compact channel_data: %w", err)
	}
	m := Member{Raw: compact.String(), UserInfo: doc.UserInfo}
	if len(doc.UserID) > 0 && string(doc.UserID) != "null" {
		m.RawUserID = doc.UserID
		m.UserID = userKey(doc.UserID)
	}
	return m, nil
}

func userKey(raw json.RawMessage) string {
	if strings.HasPrefix(string(raw), `"`) {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	return string(raw)
}

// ClientEventFrame builds the frame sent to others for a client-* event.
// Unlike server events, data stays a JSON value (pusher-js whispers objects).
func ClientEventFrame(event, channel string, data, userID json.RawMessage) []byte {
	frame := struct {
		Event   string          `json:"event"`
		Channel string          `json:"channel"`
		Data    json.RawMessage `json:"data,omitempty"`
		UserID  json.RawMessage `json:"user_id,omitempty"`
	}{event, channel, data, userID}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(frame) // RawMessage was validated when read
	return bytes.TrimRight(buf.Bytes(), "\n")
}
