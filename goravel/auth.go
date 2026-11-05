package goravel

import (
	"errors"
	"net/http"

	contractshttp "github.com/goravel/framework/contracts/http"

	"github.com/vortechron/reverb-go/broadcast"
)

const messageKey = "message"

// Logger is the part of Goravel's log the handler uses.
type Logger interface {
	Errorf(format string, args ...any)
}

// UserResolver returns the signed-in user of the request, or false when there
// is none. The value is handed to channel rules unchanged. For a session app
// it usually wraps facades.Auth().
type UserResolver func(ctx contractshttp.Context) (user any, ok bool)

// AuthConfig holds what the auth handler needs.
type AuthConfig struct {
	Credentials broadcast.Credentials
	Registry    *Registry
	User        UserResolver
	// Log is optional. Rule errors are logged once, with their cause.
	Log Logger
}

// AuthHandler answers POST /broadcasting/auth, like Laravel's Broadcast::auth.
type AuthHandler struct {
	cfg AuthConfig
}

// NewAuthHandler checks the config and builds the handler.
func NewAuthHandler(cfg AuthConfig) (*AuthHandler, error) {
	switch {
	case cfg.Credentials.Key == "" || cfg.Credentials.Secret == "":
		return nil, errors.New("reverb: auth handler needs the app key and secret")
	case cfg.Registry == nil:
		return nil, errors.New("reverb: auth handler needs a channel registry")
	case cfg.User == nil:
		return nil, errors.New("reverb: auth handler needs a user resolver")
	}

	return &AuthHandler{cfg: cfg}, nil
}

// Handle reads socket_id and channel_name (form or JSON), runs the channel
// rule and answers with the signature. 422 for a malformed request, 403 when
// there is no user or the rule says no, 500 when the rule itself fails.
func (h *AuthHandler) Handle(ctx contractshttp.Context) contractshttp.Response {
	req, err := broadcast.ParseAuthRequest(ctx.Request().Origin())
	if err != nil {
		return ctx.Response().Json(http.StatusUnprocessableEntity, map[string]string{messageKey: err.Error()})
	}
	user, ok := h.cfg.User(ctx)
	if !ok {
		return forbidden(ctx)
	}
	decision, err := h.cfg.Registry.Authorize(ctx.Context(), user, req.ChannelName)
	if err != nil {
		return h.fail(ctx, err)
	}
	if !decision.Allowed {
		return forbidden(ctx)
	}
	resp, err := h.sign(req, decision)
	if err != nil {
		return h.fail(ctx, err)
	}

	return ctx.Response().Json(http.StatusOK, resp)
}

func (h *AuthHandler) sign(req broadcast.AuthRequest, d Decision) (broadcast.AuthResponse, error) {
	if d.Member != nil {
		return h.cfg.Credentials.AuthorizePresence(req.SocketID, req.ChannelName, *d.Member)
	}

	return h.cfg.Credentials.AuthorizePrivate(req.SocketID, req.ChannelName)
}

func forbidden(ctx contractshttp.Context) contractshttp.Response {
	return ctx.Response().Json(http.StatusForbidden, map[string]string{messageKey: "Forbidden"})
}

// fail logs the real error and shows the client a plain 500.
func (h *AuthHandler) fail(ctx contractshttp.Context, err error) contractshttp.Response {
	if h.cfg.Log != nil {
		h.cfg.Log.Errorf("broadcasting auth: %v", err)
	}

	return ctx.Response().Json(http.StatusInternalServerError, map[string]string{messageKey: "Server error"})
}
