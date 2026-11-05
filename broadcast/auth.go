package broadcast

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
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

var (
	socketIDPattern = regexp.MustCompile(`^\d+\.\d+$`)
	channelPattern  = regexp.MustCompile(`^[-a-zA-Z0-9_=@,.;]{1,200}$`)
)

// ValidSocketID reports whether id has the form "digits.digits".
func ValidSocketID(id string) bool { return socketIDPattern.MatchString(id) }

// ValidChannelName applies the Pusher rule: 1 to 200 characters from [-a-zA-Z0-9_=@,.;].
func ValidChannelName(name string) bool { return channelPattern.MatchString(name) }

// validate runs before anything is signed, so ':' or other bad characters can
// never be used to forge a different string to sign.
func validate(socketID, channel string) error {
	if !ValidSocketID(socketID) {
		return errors.New("broadcast: socket_id is missing or malformed")
	}
	if !ValidChannelName(channel) {
		return errors.New("broadcast: channel_name is missing or malformed")
	}
	return nil
}

// HMACHex returns the lowercase hex HMAC-SHA256 of message.
func HMACHex(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// AuthorizePrivate signs "socket_id:channel_name" for a private channel.
func (c Credentials) AuthorizePrivate(socketID, channel string) (AuthResponse, error) {
	if err := validate(socketID, channel); err != nil {
		return AuthResponse{}, err
	}
	return AuthResponse{Auth: c.Key + ":" + HMACHex(c.Secret, socketID+":"+channel)}, nil
}

// AuthorizePresence signs "socket_id:channel_name:channel_data" for a presence channel.
func (c Credentials) AuthorizePresence(socketID, channel string, member PresenceMember) (AuthResponse, error) {
	if err := validate(socketID, channel); err != nil {
		return AuthResponse{}, err
	}
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
	r.Body = http.MaxBytesReader(nil, r.Body, maxAuthBody)
	if isJSON(r.Header.Get("Content-Type")) {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return AuthRequest{}, fmt.Errorf("broadcast: decode json auth body: %w", err)
		}
	} else {
		if err := r.ParseForm(); err != nil {
			return AuthRequest{}, fmt.Errorf("broadcast: parse form auth body: %w", err)
		}
		req = AuthRequest{SocketID: r.PostForm.Get("socket_id"), ChannelName: r.PostForm.Get("channel_name")}
	}
	if err := validate(req.SocketID, req.ChannelName); err != nil {
		return AuthRequest{}, err
	}
	return req, nil
}

// maxAuthBody is the most we read from an auth request, form or JSON.
const maxAuthBody = 64 << 10

func isJSON(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	return err == nil && media == "application/json" // ParseMediaType lowercases the type
}

// SocketIDFromRequest returns the id Echo sends in the X-Socket-Id header.
// Pass it to WithSocketID so the sender does not receive its own event.
func SocketIDFromRequest(r *http.Request) string {
	return r.Header.Get("X-Socket-Id")
}
