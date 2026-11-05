package broadcast

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// maxChannelsPerRequest is the Pusher limit that Laravel also respects.
const maxChannelsPerRequest = 100

// Config configures a Publisher.
type Config struct {
	Credentials
	// BaseURL is the server root, for example "http://127.0.0.1:8080".
	BaseURL string
	// Client is optional. A client with a 10 second timeout is used by default.
	Client *http.Client
	// Now is optional and exists so tests can fix the clock.
	Now func() time.Time
}

// Publisher sends signed events to a Pusher-protocol server.
type Publisher struct {
	cfg Config
}

// Event is one entry of a batch publish.
type Event struct {
	Channel  string
	Name     string
	Data     any
	SocketID string
}

// Option changes one publish call.
type Option func(*publishOptions)

type publishOptions struct{ socketID string }

// WithSocketID excludes one connection from receiving the event (Echo's toOthers).
func WithSocketID(id string) Option {
	return func(o *publishOptions) { o.socketID = id }
}

// NewPublisher builds a Publisher. Config is copied, so later changes do not affect it.
func NewPublisher(cfg Config) *Publisher {
	if cfg.Client == nil {
		cfg.Client = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &Publisher{cfg: cfg}
}

// Trigger publishes one event to one or more channels. Strings are sent as they
// are. Any other value is encoded as JSON, which is what Laravel does.
func (p *Publisher) Trigger(ctx context.Context, channels []string, name string, data any, opts ...Option) error {
	if len(channels) == 0 {
		return errors.New("broadcast: at least one channel is required")
	}
	if name == "" {
		return errors.New("broadcast: event name is required")
	}
	encoded, err := encodeData(data)
	if err != nil {
		return err
	}
	var o publishOptions
	for _, opt := range opts {
		opt(&o)
	}
	for start := 0; start < len(channels); start += maxChannelsPerRequest {
		end := min(start+maxChannelsPerRequest, len(channels))
		body := map[string]any{"name": name, "data": encoded, "channels": channels[start:end]}
		if o.socketID != "" {
			body["socket_id"] = o.socketID
		}
		if err := p.post(ctx, "events", body); err != nil {
			return err
		}
	}
	return nil
}

// TriggerBatch publishes many events in one request.
func (p *Publisher) TriggerBatch(ctx context.Context, events []Event) error {
	if len(events) == 0 {
		return errors.New("broadcast: at least one event is required")
	}
	batch := make([]map[string]string, 0, len(events))
	for _, e := range events {
		encoded, err := encodeData(e.Data)
		if err != nil {
			return err
		}
		item := map[string]string{"channel": e.Channel, "name": e.Name, "data": encoded}
		if e.SocketID != "" {
			item["socket_id"] = e.SocketID
		}
		batch = append(batch, item)
	}
	return p.post(ctx, "batch_events", map[string]any{"batch": batch})
}

func encodeData(data any) (string, error) {
	if s, ok := data.(string); ok {
		return s, nil
	}
	raw, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("broadcast: encode event data: %w", err)
	}
	return string(raw), nil
}

func (p *Publisher) post(ctx context.Context, endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("broadcast: encode request: %w", err)
	}
	path := "/apps/" + p.cfg.AppID + "/" + endpoint
	params := url.Values{}
	params.Set("auth_key", p.cfg.Key)
	params.Set("auth_timestamp", strconv.FormatInt(p.cfg.Now().Unix(), 10))
	params.Set("auth_version", "1.0")
	params.Set("body_md5", md5Hex(body))
	params.Set("auth_signature", RequestSignature(p.cfg.Secret, http.MethodPost, path, params, body))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.cfg.BaseURL+path+"?"+params.Encode(), bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("broadcast: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.cfg.Client.Do(req)
	if err != nil {
		return fmt.Errorf("broadcast: send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("broadcast: server answered %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}
