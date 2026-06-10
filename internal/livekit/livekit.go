// Package livekit integrates bismuth with a LiveKit SFU for voice rooms.
//
// It provides two things to the API layer:
//
//   - JoinToken: LiveKit access tokens (HS256 JWTs carrying a "video"
//     room-join grant) for browser and agent participants. Token
//     generation is pure computation and works offline.
//   - Room administration (CreateRoom, ListRooms, DeleteRoom) over
//     LiveKit's Twirp HTTP API (POST /twirp/livekit.RoomService/...),
//     using the RoomService client generated in
//     github.com/livekit/protocol/livekit — the same client that
//     github.com/livekit/server-sdk-go/v2 wraps.
//
// The package is configured with the server URL plus an API key/secret
// pair. When any of the three is missing the Manager is "disabled":
// construction still succeeds, so callers can hold a *Manager
// unconditionally (see Stub), but every operation fails fast with
// ErrDisabled.
package livekit

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	lkproto "github.com/livekit/protocol/livekit"
	"github.com/livekit/protocol/utils/xtwirp"
	"github.com/twitchtv/twirp"
)

// ErrDisabled is returned by every Manager operation when LiveKit is not
// configured. Callers should treat it as "feature off", not as a failure.
var ErrDisabled = errors.New("livekit: disabled (url, api_key and api_secret must be configured)")

const (
	// defaultJoinTokenTTL matches the default validity used by LiveKit's
	// own token generator (protocol/auth).
	defaultJoinTokenTTL = 6 * time.Hour
	// adminTokenTTL bounds the short-lived service tokens attached to
	// RoomService API requests.
	adminTokenTTL = 10 * time.Minute
)

// Config holds LiveKit connection parameters.
type Config struct {
	URL       string `yaml:"url"` // e.g. "wss://bismuth.livekit.cloud" or "http://localhost:7880"
	APIKey    string `yaml:"api_key"`
	APISecret string `yaml:"api_secret"`
}

// Enabled reports whether the configuration is complete enough to use
// LiveKit, i.e. URL, APIKey and APISecret are all non-empty.
func (c Config) Enabled() bool {
	return c.URL != "" && c.APIKey != "" && c.APISecret != ""
}

// RoomInfo is a transport-agnostic summary of a LiveKit room.
type RoomInfo struct {
	Name            string `json:"name"`
	NumParticipants int    `json:"num_participants"`
	CreatedAt       int64  `json:"created_at"` // unix seconds
}

// Manager issues participant tokens and administers LiveKit rooms.
// The zero-config Manager returned by Stub (or NewManager with an
// incomplete Config) is safe to use; its methods return ErrDisabled.
type Manager struct {
	cfg  Config
	room lkproto.RoomService // generated Twirp client; nil when disabled
}

// NewManager creates a Manager from cfg. It never fails: with an
// incomplete configuration the Manager is disabled rather than nil.
func NewManager(cfg Config) *Manager {
	m := &Manager{cfg: cfg}
	if cfg.Enabled() {
		m.room = lkproto.NewRoomServiceJSONClient(
			toHTTPURL(cfg.URL), &http.Client{}, xtwirp.DefaultClientOptions()...)
	}
	return m
}

// Stub returns a disabled Manager so callers can hold a *Manager
// unconditionally even when LiveKit is not configured.
func Stub() *Manager { return NewManager(Config{}) }

// Enabled reports whether this Manager has a complete LiveKit
// configuration. It is safe to call on a nil Manager.
func (m *Manager) Enabled() bool { return m != nil && m.cfg.Enabled() }

// JoinToken returns a signed LiveKit access token granting `identity`
// permission to join `room`. ttl <= 0 falls back to the LiveKit default
// of 6 hours. Token generation is local-only and needs no network.
func (m *Manager) JoinToken(room, identity string, ttl time.Duration) (string, error) {
	if !m.Enabled() {
		return "", ErrDisabled
	}
	if room == "" {
		return "", errors.New("livekit: room name must not be empty")
	}
	if identity == "" {
		return "", errors.New("livekit: participant identity must not be empty")
	}
	if ttl <= 0 {
		ttl = defaultJoinTokenTTL
	}
	return signAccessToken(m.cfg.APIKey, m.cfg.APISecret, claimGrants{
		Identity: identity,
		Video:    &videoGrant{RoomJoin: true, Room: room},
	}, ttl, time.Now())
}

// CreateRoom creates the named room on the LiveKit server. The call is
// idempotent: LiveKit returns the existing room when one with the same
// name is already live.
func (m *Manager) CreateRoom(ctx context.Context, name string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	if name == "" {
		return errors.New("livekit: room name must not be empty")
	}
	ctx, err := m.adminContext(ctx, videoGrant{RoomCreate: true})
	if err != nil {
		return err
	}
	if _, err := m.room.CreateRoom(ctx, &lkproto.CreateRoomRequest{Name: name}); err != nil {
		return fmt.Errorf("livekit: create room %q: %w", name, err)
	}
	return nil
}

// ListRooms returns the rooms currently live on the LiveKit server.
func (m *Manager) ListRooms(ctx context.Context) ([]RoomInfo, error) {
	if !m.Enabled() {
		return nil, ErrDisabled
	}
	ctx, err := m.adminContext(ctx, videoGrant{RoomList: true})
	if err != nil {
		return nil, err
	}
	resp, err := m.room.ListRooms(ctx, &lkproto.ListRoomsRequest{})
	if err != nil {
		return nil, fmt.Errorf("livekit: list rooms: %w", err)
	}
	rooms := make([]RoomInfo, 0, len(resp.GetRooms()))
	for _, r := range resp.GetRooms() {
		rooms = append(rooms, RoomInfo{
			Name:            r.GetName(),
			NumParticipants: int(r.GetNumParticipants()),
			CreatedAt:       r.GetCreationTime(),
		})
	}
	return rooms, nil
}

// DeleteRoom ends the named room, disconnecting all participants.
func (m *Manager) DeleteRoom(ctx context.Context, name string) error {
	if !m.Enabled() {
		return ErrDisabled
	}
	if name == "" {
		return errors.New("livekit: room name must not be empty")
	}
	// LiveKit gates DeleteRoom behind the roomCreate grant, mirroring
	// lksdk's RoomServiceClient.DeleteRoom.
	ctx, err := m.adminContext(ctx, videoGrant{RoomCreate: true})
	if err != nil {
		return err
	}
	if _, err := m.room.DeleteRoom(ctx, &lkproto.DeleteRoomRequest{Room: name}); err != nil {
		return fmt.Errorf("livekit: delete room %q: %w", name, err)
	}
	return nil
}

// adminContext attaches a short-lived service token carrying the given
// grant to the outgoing Twirp request headers, mirroring the auth
// behavior of lksdk's RoomServiceClient.
func (m *Manager) adminContext(ctx context.Context, grant videoGrant) (context.Context, error) {
	token, err := signAccessToken(m.cfg.APIKey, m.cfg.APISecret,
		claimGrants{Video: &grant}, adminTokenTTL, time.Now())
	if err != nil {
		return nil, err
	}
	header := make(http.Header)
	header.Set("Authorization", "Bearer "+token)
	return twirp.WithHTTPRequestHeaders(ctx, header)
}

// toHTTPURL converts LiveKit websocket URLs (ws://, wss://) to their
// HTTP equivalents for the Twirp API; http(s) URLs pass through
// unchanged. Equivalent of lksdk's signalling.ToHttpURL.
func toHTTPURL(url string) string {
	if strings.HasPrefix(url, "ws") {
		return strings.Replace(url, "ws", "http", 1)
	}
	return url
}
