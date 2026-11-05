package goravel

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vortechron/reverb-go/broadcast"
)

func allow(context.Context, any, map[string]string) (bool, *broadcast.PresenceMember, error) {
	return true, nil, nil
}

func TestRegistryMatchesPatternsAndPassesParams(t *testing.T) {
	r := NewRegistry()
	var got map[string]string
	require.NoError(t, r.Channel("users.{id}", func(_ context.Context, user any, params map[string]string) (bool, *broadcast.PresenceMember, error) {
		got = params

		return user == "u5" && params["id"] == "5", nil, nil
	}))

	d, err := r.Authorize(context.Background(), "u5", "private-users.5")
	require.NoError(t, err)
	assert.True(t, d.Allowed)
	assert.Equal(t, map[string]string{"id": "5"}, got)

	d, err = r.Authorize(context.Background(), "u5", "private-users.6")
	require.NoError(t, err)
	assert.True(t, d.Allowed == (got["id"] == "5"), "callback decides")
	assert.False(t, d.Allowed)
}

func TestRegistryUnknownOrPublicChannelIsDenied(t *testing.T) {
	r := NewRegistry()
	require.NoError(t, r.Channel("users.{id}", allow))

	for _, name := range []string{"private-orders.1", "users.5", "private-users.5.extra", "private-users."} {
		d, err := r.Authorize(context.Background(), "u", name)
		require.NoError(t, err, name)
		assert.False(t, d.Allowed, name)
	}
}

func TestRegistryPresenceNeedsAMember(t *testing.T) {
	r := NewRegistry()
	member := &broadcast.PresenceMember{UserID: 7, UserInfo: map[string]any{"name": "Ana"}}
	require.NoError(t, r.Channel("room.{room}", func(context.Context, any, map[string]string) (bool, *broadcast.PresenceMember, error) {
		return true, member, nil
	}))
	require.NoError(t, r.Channel("bad.{room}", allow))

	d, err := r.Authorize(context.Background(), "u", "presence-room.1")
	require.NoError(t, err)
	assert.True(t, d.Allowed)
	assert.Equal(t, member, d.Member)

	_, err = r.Authorize(context.Background(), "u", "presence-bad.1")
	assert.ErrorIs(t, err, ErrNoPresenceMember)

	d, err = r.Authorize(context.Background(), "u", "private-room.1")
	require.NoError(t, err)
	assert.Nil(t, d.Member, "private channel never carries a member")
}

func TestRegistryCallbackErrorIsWrapped(t *testing.T) {
	r := NewRegistry()
	boom := errors.New("db down")
	require.NoError(t, r.Channel("a", func(context.Context, any, map[string]string) (bool, *broadcast.PresenceMember, error) {
		return true, nil, boom
	}))

	d, err := r.Authorize(context.Background(), "u", "private-a")
	assert.ErrorIs(t, err, boom)
	assert.False(t, d.Allowed, "an error never allows")
}

func TestRegistryRejectsBadRules(t *testing.T) {
	r := NewRegistry()
	assert.Error(t, r.Channel("", allow))
	assert.Error(t, r.Channel("x.{id}", nil))
	assert.Error(t, r.Channel("x.{}", allow))
	assert.Error(t, r.Channel("x.{id}.{id}", allow))
	require.NoError(t, r.Channel("x.{id}", allow))
	assert.Error(t, r.Channel("x.{id}", allow), "same pattern twice")
}

func TestRegistryFirstMatchingRuleWins(t *testing.T) {
	r := NewRegistry()
	deny := func(context.Context, any, map[string]string) (bool, *broadcast.PresenceMember, error) {
		return false, nil, nil
	}
	require.NoError(t, r.Channel("orders.{id}", deny))
	require.NoError(t, r.Channel("orders.{other}", allow))

	d, err := r.Authorize(context.Background(), "u", "private-orders.1")
	require.NoError(t, err)
	assert.False(t, d.Allowed)
}
