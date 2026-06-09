// Package livekit provides the LiveKit SFU integration for bismuth voice.
//
// V2 approach (LiveKit room-based):
//
//   1. Browser joins a LiveKit room via LiveKit client SDK.
//   2. Audio track is published to the room.
//   3. Server-side bot (this package) subscribes to the track,
//      runs STT on the audio stream in real-time.
//   4. Parsed commands are dispatched to bismuth API.
//   5. Response text is synthesized via TTS and published back
//      as an audio track in the same room.
//
// This is a STUB — the actual LiveKit SDK integration requires:
//   - go get github.com/livekit/server-sdk-go
//   - LiveKit server deployed (self-hosted or cloud)
//   - API key/secret from LiveKit
//
// For now, the V1 HTTP-based voice gateway (voice.go) is the default.
package livekit

import (
	"context"
	"fmt"
	"time"

	"github.com/biodoia/bismuth/internal/logger"
)

// Config holds LiveKit connection parameters.
type Config struct {
	Host      string `yaml:"host"`       // e.g. "wss://bismuth.livekit.cloud"
	APIKey    string `yaml:"api_key"`
	APISecret string `yaml:"api_secret"`
	RoomName  string `yaml:"room_name"`  // default: "bismuth"
}

// RoomManager manages voice rooms for agent conversations.
type RoomManager struct {
	cfg Config
}

// NewRoomManager creates a LiveKit room manager.
// Returns an error if config is incomplete (stub mode).
func NewRoomManager(cfg Config) (*RoomManager, error) {
	if cfg.Host == "" || cfg.APIKey == "" || cfg.APISecret == "" {
		return nil, fmt.Errorf("livekit: config incomplete (host/api_key/api_secret required)")
	}
	return &RoomManager{cfg: cfg}, nil
}

// RoomInfo describes a live voice room.
type RoomInfo struct {
	Name      string    `json:"name"`
	AgentID   string    `json:"agent_id"`
	JoinedAt  time.Time `json:"joined_at"`
	State     string    `json:"state"` // "active" | "ended"
}

// CreateRoom creates a new voice room for the given agent.
// STUB: returns placeholder info.
func (rm *RoomManager) CreateRoom(ctx context.Context, agentID string) (*RoomInfo, error) {
	name := "bismuth-" + agentID
	logger.Info("livekit: creating room (stub)", "room", name, "agent_id", agentID)
	return &RoomInfo{
		Name:     name,
		AgentID:  agentID,
		JoinedAt: time.Now().UTC(),
		State:    "active",
	}, nil
}

// EndRoom ends a voice room.
// STUB: logs only.
func (rm *RoomManager) EndRoom(ctx context.Context, roomName string) error {
	logger.Info("livekit: ending room (stub)", "room", roomName)
	return nil
}

// ListRooms lists active voice rooms.
// STUB: returns empty.
func (rm *RoomManager) ListRooms(ctx context.Context) ([]RoomInfo, error) {
	return nil, nil
}

// JoinToken generates a LiveKit access token for a participant.
// STUB: returns error.
func (rm *RoomManager) JoinToken(roomName, participantID string, canPublish bool) (string, error) {
	return "", fmt.Errorf("livekit: JoinToken not implemented (stub)")
}
