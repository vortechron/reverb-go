package server

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/vortechron/reverb-go/internal/wire"
)

func equalStrings(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// eventItem is one event of /events or /batch_events.
type eventItem struct {
	Name     *string         `json:"name"`
	Data     json.RawMessage `json:"data"`
	Channels []string        `json:"channels"`
	Channel  *string         `json:"channel"`
	SocketID *string         `json:"socket_id"`
	Info     *string         `json:"info"`
}

type validationErrors map[string][]string

func (v validationErrors) add(field, text string) { v[field] = append(v[field], text) }

// check applies the same rules as Reverb's validators.
func (e eventItem) check(prefix string, errs validationErrors) {
	if e.Name == nil {
		errs.add(prefix+"name", "The "+prefix+"name field is required.")
	}
	if len(bytes.TrimSpace(e.Data)) == 0 || e.Data[0] != '"' {
		errs.add(prefix+"data", "The "+prefix+"data field is required and must be a string.")
	}
	if e.Channel == nil && len(e.Channels) == 0 {
		errs.add(prefix+"channels", "The "+prefix+"channels field is required when "+prefix+"channel is not present.")
	}
}

func (e eventItem) dataString() string {
	var s string
	_ = json.Unmarshal(e.Data, &s) // shape checked in check()
	return s
}

func (e eventItem) channelList() []string {
	if len(e.Channels) > 0 {
		return e.Channels
	}
	if e.Channel != nil {
		return []string{*e.Channel}
	}
	return nil
}

func (e eventItem) socketID() string {
	if e.SocketID == nil {
		return ""
	}
	return *e.SocketID
}

func decodeBody(body []byte, into any) (int, any) {
	if err := json.Unmarshal(body, into); err != nil {
		var syntax *json.SyntaxError
		if asSyntax(err, &syntax) || len(body) == 0 {
			return http.StatusBadRequest, msg("Invalid JSON body.")
		}
		return http.StatusUnprocessableEntity, validationErrors{"body": {"The request body is invalid."}}
	}
	return 0, nil
}

func (s *Server) events(c *call) (int, any) {
	var item eventItem
	if status, payload := decodeBody(c.body, &item); status != 0 {
		return status, payload
	}
	errs := validationErrors{}
	item.check("", errs)
	if len(errs) > 0 {
		return http.StatusUnprocessableEntity, errs
	}
	channels := item.channelList()
	for _, ch := range channels {
		s.publish(c.app, ch, wire.Event(*item.Name, ch, item.dataString()), item.socketID())
	}
	if item.Info != nil {
		out := map[string]map[string]any{}
		for _, ch := range channels {
			out[ch] = channelInfo(c.hub, ch, *item.Info)
		}
		return http.StatusOK, map[string]any{"channels": out}
	}
	return http.StatusOK, struct{}{}
}

func (s *Server) batchEvents(c *call) (int, any) {
	var doc struct {
		Batch []eventItem `json:"batch"`
	}
	if status, payload := decodeBody(c.body, &doc); status != 0 {
		return status, payload
	}
	errs := validationErrors{}
	if doc.Batch == nil {
		errs.add("batch", "The batch field is required.")
	}
	for i, item := range doc.Batch {
		prefix := "batch." + strconv.Itoa(i) + "."
		item.check(prefix, errs)
		if item.Channel == nil {
			errs.add(prefix+"channel", fmt.Sprintf("The %schannel field is required.", prefix))
		}
	}
	if len(errs) > 0 {
		return http.StatusUnprocessableEntity, errs
	}
	infos := make([]map[string]any, len(doc.Batch))
	withInfo := false
	for i, item := range doc.Batch {
		s.publish(c.app, *item.Channel, wire.Event(*item.Name, *item.Channel, item.dataString()), item.socketID())
		infos[i] = map[string]any{}
		if item.Info != nil {
			withInfo = true
			infos[i] = channelInfo(c.hub, *item.Channel, *item.Info)
		}
	}
	if withInfo {
		return http.StatusOK, map[string]any{"batch": infos}
	}
	return http.StatusOK, map[string]any{"batch": struct{}{}}
}

func (s *Server) terminate(c *call) (int, any) {
	s.terminateUser(c.app, c.r.PathValue("userId"))
	return http.StatusOK, struct{}{}
}
