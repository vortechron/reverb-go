package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/vortechron/reverb-go/broadcast"
	"github.com/vortechron/reverb-go/internal/apps"
	"github.com/vortechron/reverb-go/internal/hub"
)

// maxClockSkew is the Pusher rule for auth_timestamp.
const maxClockSkew = 600 * time.Second

// call is one verified API request.
type call struct {
	app   apps.App
	hub   *hub.Hub
	query url.Values
	body  []byte
	r     *http.Request
}

type apiHandler func(c *call) (status int, payload any)

type apiError struct {
	status  int
	payload any
}

func (s *Server) routeAPI(mux *http.ServeMux, p string) {
	mux.HandleFunc("POST "+p+"/apps/{appId}/events", s.api(s.events))
	mux.HandleFunc("POST "+p+"/apps/{appId}/batch_events", s.api(s.batchEvents))
	mux.HandleFunc("GET "+p+"/apps/{appId}/connections", s.api(s.connections))
	mux.HandleFunc("GET "+p+"/apps/{appId}/channels", s.api(s.channels))
	mux.HandleFunc("GET "+p+"/apps/{appId}/channels/{channel}", s.api(s.channel))
	mux.HandleFunc("GET "+p+"/apps/{appId}/channels/{channel}/users", s.api(s.channelUsers))
	mux.HandleFunc("POST "+p+"/apps/{appId}/users/{userId}/terminate_connections", s.api(s.terminate))
	mux.HandleFunc("GET "+p+"/up", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"health": "OK"})
	})
}

// api wraps a handler with app lookup, body limit and signature check.
func (s *Server) api(h apiHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, aerr := s.authenticate(w, r)
		if aerr != nil {
			writeJSON(w, aerr.status, aerr.payload)
			return
		}
		status, payload := h(c)
		writeJSON(w, status, payload)
	}
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (*call, *apiError) {
	app, ok := s.opts.Apps.ByID(r.PathValue("appId"))
	if !ok {
		return nil, &apiError{http.StatusNotFound, msg("No matching application for ID [" + r.PathValue("appId") + "].")}
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, s.opts.MaxRequestSize))
	if err != nil {
		return nil, &apiError{http.StatusRequestEntityTooLarge, msg("Request body too large.")}
	}
	query := r.URL.Query()
	if err := s.verifySignature(app, r, query, body); err != nil {
		return nil, &apiError{http.StatusUnauthorized, msg(err.Error())}
	}
	return &call{app: app, hub: s.hubs[app.ID], query: query, body: body, r: r}, nil
}

func (s *Server) verifySignature(app apps.App, r *http.Request, query url.Values, body []byte) error {
	params := url.Values{}
	for k, v := range query {
		switch k {
		case "auth_signature", "body_md5", "appId", "appKey", "channelName":
		default:
			params[k] = v
		}
	}
	path := r.URL.EscapedPath()
	if s.opts.Path != "" {
		path = "/" + strings.TrimLeft(strings.TrimPrefix(path, s.opts.Path), "/")
	}
	want := broadcast.RequestSignature(app.Secret, r.Method, path, params, body)
	if !equalStrings(want, query.Get("auth_signature")) {
		return errors.New("Authentication signature invalid.") //nolint:revive,staticcheck // message mirrors Reverb.
	}
	ts, err := strconv.ParseInt(query.Get("auth_timestamp"), 10, 64)
	if err != nil || absDuration(time.Since(time.Unix(ts, 0))) > maxClockSkew {
		return errors.New("Authentication timestamp invalid or too old.") //nolint:revive,staticcheck // see above.
	}
	return nil
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func msg(text string) map[string]string { return map[string]string{"error": text} }

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload) // the client may have gone; nothing to do
}

// ConnectionCount returns the open connections of one app.
func (s *Server) ConnectionCount(appID string) int {
	if h, ok := s.hubs[appID]; ok {
		return h.ConnectionCount()
	}
	return 0
}
