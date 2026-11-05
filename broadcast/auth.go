package broadcast

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
)

// Credentials identify one app on the Pusher-protocol server.
type Credentials struct {
	AppID  string
	Key    string
	Secret string
}

// AuthResponse is the JSON body to return from /broadcasting/auth.
type AuthResponse struct {
	Auth        string `json:"auth"`
	ChannelData string `json:"channel_data,omitempty"`
}

// PresenceMember is the user that joins a presence channel.
// UserID is kept as given, so an int stays an int (Laravel sends ints).
type PresenceMember struct {
	UserID   any
	UserInfo any
}

// AuthRequest holds the two fields a client posts to the auth endpoint.
type AuthRequest struct {
	SocketID    string `json:"socket_id"`
	ChannelName string `json:"channel_name"`
}

var socketIDPattern = regexp.MustCompile(`^\d+\.\d+$`)

// HMACHex returns the lowercase hex HMAC-SHA256 of message.
func HMACHex(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// AuthorizePrivate signs "socket_id:channel_name" for a private channel.
func (c Credentials) AuthorizePrivate(socketID, channel string) AuthResponse {
	return AuthResponse{Auth: c.Key + ":" + HMACHex(c.Secret, socketID+":"+channel)}
}

// AuthorizePresence signs "socket_id:channel_name:channel_data" for a presence channel.
func (c Credentials) AuthorizePresence(socketID, channel string, member PresenceMember) (AuthResponse, error) {
	if member.UserID == nil || member.UserID == "" {
		return AuthResponse{}, errors.New("broadcast: presence member needs a user_id")
	}
	data, err := encodeChannelData(member)
	if err != nil {
		return AuthResponse{}, fmt.Errorf("broadcast: encode channel_data: %w", err)
	}
	return AuthResponse{
		Auth:        c.Key + ":" + HMACHex(c.Secret, socketID+":"+channel+":"+data),
		ChannelData: data,
	}, nil
}

func encodeChannelData(member PresenceMember) (string, error) {
	payload := map[string]any{"user_id": member.UserID}
	if member.UserInfo != nil {
		payload["user_info"] = member.UserInfo
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(payload); err != nil {
		return "", err
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// ParseAuthRequest reads socket_id and channel_name from a form body or a JSON body.
func ParseAuthRequest(r *http.Request) (AuthRequest, error) {
	var req AuthRequest
	if isJSON(r.Header.Get("Content-Type")) {
		if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<16)).Decode(&req); err != nil {
			return AuthRequest{}, fmt.Errorf("broadcast: decode json auth body: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return AuthRequest{}, fmt.Errorf("broadcast: parse form auth body: %w", err)
		}
		req = AuthRequest{SocketID: r.PostForm.Get("socket_id"), ChannelName: r.PostForm.Get("channel_name")}
	}
	if req.ChannelName == "" {
		return AuthRequest{}, errors.New("broadcast: channel_name is required")
	}
	if !socketIDPattern.MatchString(req.SocketID) {
		return AuthRequest{}, errors.New("broadcast: socket_id is missing or malformed")
	}
	return req, nil
}

func isJSON(contentType string) bool {
	return len(contentType) >= 16 && contentType[:16] == "application/json"
}

// SocketIDFromRequest returns the id Echo sends in the X-Socket-Id header.
// Pass it to WithSocketID so the sender does not receive its own event.
func SocketIDFromRequest(r *http.Request) string {
	return r.Header.Get("X-Socket-Id")
}
