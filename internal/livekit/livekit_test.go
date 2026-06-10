package livekit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	lkproto "github.com/livekit/protocol/livekit"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	testURL    = "wss://livekit.example.test"
	testKey    = "APIbismuthTest"
	testSecret = "sup3r-secret-livekit-key-for-tests"
)

// tokenPayload mirrors the JWT payload shape for test-side decoding.
type tokenPayload struct {
	Iss      string `json:"iss"`
	Sub      string `json:"sub"`
	Nbf      int64  `json:"nbf"`
	Exp      int64  `json:"exp"`
	Identity string `json:"identity"`
	Video    struct {
		RoomCreate bool   `json:"roomCreate"`
		RoomList   bool   `json:"roomList"`
		RoomAdmin  bool   `json:"roomAdmin"`
		RoomJoin   bool   `json:"roomJoin"`
		Room       string `json:"room"`
	} `json:"video"`
}

// parseToken decodes a compact JWT, verifying the JOSE header and the
// HS256 signature against secret, and returns the decoded payload.
func parseToken(t *testing.T, token, secret string) tokenPayload {
	t.Helper()

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3: %q", len(parts), token)
	}

	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode header: %v", err)
	}
	var header struct {
		Alg string `json:"alg"`
		Typ string `json:"typ"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	if header.Alg != "HS256" || header.Typ != "JWT" {
		t.Fatalf("header = %+v, want alg=HS256 typ=JWT", header)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(parts[0] + "." + parts[1]))
	wantSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(wantSig), []byte(parts[2])) {
		t.Fatalf("signature mismatch: got %q, want %q", parts[2], wantSig)
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var payload tokenPayload
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload
}

func TestConfigEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  Config
		want bool
	}{
		{"all set", Config{URL: testURL, APIKey: testKey, APISecret: testSecret}, true},
		{"empty", Config{}, false},
		{"missing url", Config{APIKey: testKey, APISecret: testSecret}, false},
		{"missing api key", Config{URL: testURL, APISecret: testSecret}, false},
		{"missing api secret", Config{URL: testURL, APIKey: testKey}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cfg.Enabled(); got != tt.want {
				t.Errorf("Config.Enabled() = %v, want %v", got, tt.want)
			}
			if got := NewManager(tt.cfg).Enabled(); got != tt.want {
				t.Errorf("Manager.Enabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestJoinToken(t *testing.T) {
	m := NewManager(Config{URL: testURL, APIKey: testKey, APISecret: testSecret})

	tests := []struct {
		name    string
		ttl     time.Duration
		wantTTL int64 // expected exp-nbf in seconds
	}{
		{"explicit ttl", 45 * time.Minute, 45 * 60},
		{"zero ttl uses default", 0, int64(defaultJoinTokenTTL / time.Second)},
		{"negative ttl uses default", -time.Minute, int64(defaultJoinTokenTTL / time.Second)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := m.JoinToken("ops-room", "agent-7", tt.ttl)
			if err != nil {
				t.Fatalf("JoinToken() error = %v", err)
			}

			payload := parseToken(t, token, testSecret)
			if payload.Iss != testKey {
				t.Errorf("iss = %q, want %q", payload.Iss, testKey)
			}
			if payload.Sub != "agent-7" {
				t.Errorf("sub = %q, want %q", payload.Sub, "agent-7")
			}
			if payload.Identity != "agent-7" {
				t.Errorf("identity = %q, want %q", payload.Identity, "agent-7")
			}
			if !payload.Video.RoomJoin {
				t.Error("video.roomJoin = false, want true")
			}
			if payload.Video.Room != "ops-room" {
				t.Errorf("video.room = %q, want %q", payload.Video.Room, "ops-room")
			}
			if got := payload.Exp - payload.Nbf; got != tt.wantTTL {
				t.Errorf("exp-nbf = %ds, want %ds", got, tt.wantTTL)
			}
			if drift := payload.Nbf - time.Now().Unix(); drift < -5 || drift > 5 {
				t.Errorf("nbf = %d, want within 5s of now (%d)", payload.Nbf, time.Now().Unix())
			}
		})
	}
}

func TestJoinTokenValidation(t *testing.T) {
	m := NewManager(Config{URL: testURL, APIKey: testKey, APISecret: testSecret})

	tests := []struct {
		name     string
		room     string
		identity string
	}{
		{"empty room", "", "agent-7"},
		{"empty identity", "ops-room", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := m.JoinToken(tt.room, tt.identity, time.Minute)
			if err == nil {
				t.Fatal("JoinToken() error = nil, want non-nil")
			}
			if errors.Is(err, ErrDisabled) {
				t.Errorf("JoinToken() error = ErrDisabled, want validation error")
			}
		})
	}
}

func TestDisabledOperations(t *testing.T) {
	managers := []struct {
		name string
		m    *Manager
	}{
		{"nil manager", nil},
		{"stub", Stub()},
		{"missing credentials", NewManager(Config{URL: testURL})},
		{"missing url", NewManager(Config{APIKey: testKey, APISecret: testSecret})},
	}
	ops := []struct {
		name string
		call func(m *Manager) error
	}{
		{"JoinToken", func(m *Manager) error {
			_, err := m.JoinToken("room", "id", time.Minute)
			return err
		}},
		{"CreateRoom", func(m *Manager) error {
			return m.CreateRoom(context.Background(), "room")
		}},
		{"ListRooms", func(m *Manager) error {
			_, err := m.ListRooms(context.Background())
			return err
		}},
		{"DeleteRoom", func(m *Manager) error {
			return m.DeleteRoom(context.Background(), "room")
		}},
	}
	for _, mc := range managers {
		for _, op := range ops {
			t.Run(mc.name+"/"+op.name, func(t *testing.T) {
				if err := op.call(mc.m); !errors.Is(err, ErrDisabled) {
					t.Errorf("%s error = %v, want ErrDisabled", op.name, err)
				}
			})
		}
	}
}

func TestToHTTPURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"ws://localhost:7880", "http://localhost:7880"},
		{"wss://livekit.example.test", "https://livekit.example.test"},
		{"http://localhost:7880", "http://localhost:7880"},
		{"https://livekit.example.test", "https://livekit.example.test"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := toHTTPURL(tt.in); got != tt.want {
				t.Errorf("toHTTPURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// recordedCall captures one request received by the fake LiveKit server.
type recordedCall struct {
	path        string
	method      string
	contentType string
	authz       string
	body        []byte
}

// fakeLiveKit is a hermetic httptest-backed stand-in for the LiveKit
// RoomService Twirp endpoint.
type fakeLiveKit struct {
	mu    sync.Mutex
	calls []recordedCall
}

func (f *fakeLiveKit) record(c recordedCall) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, c)
}

// lastCall returns the most recent request whose path ends in method.
func (f *fakeLiveKit) lastCall(t *testing.T, method string) recordedCall {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := len(f.calls) - 1; i >= 0; i-- {
		if strings.HasSuffix(f.calls[i].path, "/"+method) {
			return f.calls[i]
		}
	}
	t.Fatalf("no recorded call for method %q", method)
	return recordedCall{}
}

func (f *fakeLiveKit) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.record(recordedCall{
			path:        r.URL.Path,
			method:      r.Method,
			contentType: r.Header.Get("Content-Type"),
			authz:       r.Header.Get("Authorization"),
			body:        body,
		})

		respond := func(b []byte, err error) {
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(b)
		}

		switch r.URL.Path {
		case "/twirp/livekit.RoomService/CreateRoom":
			var req lkproto.CreateRoomRequest
			if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(body, &req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			respond(protojson.Marshal(&lkproto.Room{
				Sid:          "RM_fake",
				Name:         req.GetName(),
				CreationTime: 1718000000,
			}))
		case "/twirp/livekit.RoomService/ListRooms":
			respond(protojson.Marshal(&lkproto.ListRoomsResponse{
				Rooms: []*lkproto.Room{
					{Sid: "RM_1", Name: "bismuth-ops", NumParticipants: 3, CreationTime: 1717999000},
					{Sid: "RM_2", Name: "bismuth-dev", NumParticipants: 0, CreationTime: 1717999500},
				},
			}))
		case "/twirp/livekit.RoomService/DeleteRoom":
			respond(protojson.Marshal(&lkproto.DeleteRoomResponse{}))
		default:
			t.Errorf("unexpected request path %q", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
}

// checkAdminAuth verifies the Authorization header of a recorded call:
// bearer JWT signed with the API secret, issued by the API key, carrying
// the expected video grant.
func checkAdminAuth(t *testing.T, c recordedCall, wantGrant func(t *testing.T, p tokenPayload)) {
	t.Helper()
	const prefix = "Bearer "
	if !strings.HasPrefix(c.authz, prefix) {
		t.Fatalf("Authorization = %q, want Bearer token", c.authz)
	}
	payload := parseToken(t, strings.TrimPrefix(c.authz, prefix), testSecret)
	if payload.Iss != testKey {
		t.Errorf("admin token iss = %q, want %q", payload.Iss, testKey)
	}
	wantGrant(t, payload)
}

func TestRoomServiceOps(t *testing.T) {
	fake := &fakeLiveKit{}
	ts := httptest.NewServer(fake.handler(t))
	defer ts.Close()

	// Use a ws:// URL pointing at the fake to exercise the ws->http
	// conversion done for Twirp calls.
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	m := NewManager(Config{URL: wsURL, APIKey: testKey, APISecret: testSecret})
	ctx := context.Background()

	t.Run("CreateRoom", func(t *testing.T) {
		if err := m.CreateRoom(ctx, "bismuth-ops"); err != nil {
			t.Fatalf("CreateRoom() error = %v", err)
		}
		call := fake.lastCall(t, "CreateRoom")
		if call.method != http.MethodPost {
			t.Errorf("method = %q, want POST", call.method)
		}
		if call.contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", call.contentType)
		}
		var req lkproto.CreateRoomRequest
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(call.body, &req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.GetName() != "bismuth-ops" {
			t.Errorf("request name = %q, want %q", req.GetName(), "bismuth-ops")
		}
		checkAdminAuth(t, call, func(t *testing.T, p tokenPayload) {
			if !p.Video.RoomCreate {
				t.Error("admin token video.roomCreate = false, want true")
			}
		})
	})

	t.Run("ListRooms", func(t *testing.T) {
		rooms, err := m.ListRooms(ctx)
		if err != nil {
			t.Fatalf("ListRooms() error = %v", err)
		}
		want := []RoomInfo{
			{Name: "bismuth-ops", NumParticipants: 3, CreatedAt: 1717999000},
			{Name: "bismuth-dev", NumParticipants: 0, CreatedAt: 1717999500},
		}
		if !slices.Equal(rooms, want) {
			t.Errorf("ListRooms() = %+v, want %+v", rooms, want)
		}
		call := fake.lastCall(t, "ListRooms")
		if call.method != http.MethodPost {
			t.Errorf("method = %q, want POST", call.method)
		}
		if call.contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", call.contentType)
		}
		checkAdminAuth(t, call, func(t *testing.T, p tokenPayload) {
			if !p.Video.RoomList {
				t.Error("admin token video.roomList = false, want true")
			}
		})
	})

	t.Run("DeleteRoom", func(t *testing.T) {
		if err := m.DeleteRoom(ctx, "bismuth-ops"); err != nil {
			t.Fatalf("DeleteRoom() error = %v", err)
		}
		call := fake.lastCall(t, "DeleteRoom")
		var req lkproto.DeleteRoomRequest
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(call.body, &req); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		if req.GetRoom() != "bismuth-ops" {
			t.Errorf("request room = %q, want %q", req.GetRoom(), "bismuth-ops")
		}
		checkAdminAuth(t, call, func(t *testing.T, p tokenPayload) {
			if !p.Video.RoomCreate {
				t.Error("admin token video.roomCreate = false, want true")
			}
		})
	})
}

func TestRoomServiceTwirpError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"unavailable","msg":"room service down"}`))
	}))
	defer ts.Close()

	m := NewManager(Config{URL: ts.URL, APIKey: testKey, APISecret: testSecret})

	err := m.CreateRoom(context.Background(), "bismuth-ops")
	if err == nil {
		t.Fatal("CreateRoom() error = nil, want twirp error")
	}
	if errors.Is(err, ErrDisabled) {
		t.Error("CreateRoom() error is ErrDisabled, want twirp error")
	}
	if !strings.Contains(err.Error(), "room service down") {
		t.Errorf("CreateRoom() error = %q, want it to contain %q", err, "room service down")
	}
}
