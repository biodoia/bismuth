package livekit

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// This file produces standard LiveKit access tokens: HS256-signed JWTs
// whose payload carries the LiveKit grant claims. The wire format is
// identical to github.com/livekit/protocol/auth
// (NewAccessToken(key, secret).SetIdentity(...).SetVideoGrant(...).ToJWT()),
// implemented with the standard library only so token generation has no
// extra dependencies and always works offline.

// videoGrant mirrors the JSON shape of auth.VideoGrant (the subset
// bismuth uses).
type videoGrant struct {
	RoomCreate bool   `json:"roomCreate,omitempty"`
	RoomList   bool   `json:"roomList,omitempty"`
	RoomAdmin  bool   `json:"roomAdmin,omitempty"`
	RoomJoin   bool   `json:"roomJoin,omitempty"`
	Room       string `json:"room,omitempty"`
}

// claimGrants mirrors the JSON shape of auth.ClaimGrants (the subset
// bismuth uses).
type claimGrants struct {
	Identity string      `json:"identity,omitempty"`
	Video    *videoGrant `json:"video,omitempty"`
}

// tokenClaims is the flattened JWT payload: registered claims plus the
// LiveKit grant claims, exactly as protocol/auth serializes them.
type tokenClaims struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub,omitempty"`
	NotBefore int64  `json:"nbf"`
	ExpiresAt int64  `json:"exp"`
	claimGrants
}

// jwtHeader is the fixed JOSE header for LiveKit tokens, pre-encoded in
// base64url (raw, no padding): {"alg":"HS256","typ":"JWT"}.
var jwtHeader = base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// signAccessToken builds and signs a LiveKit-compatible HS256 JWT valid
// from now until now+ttl.
func signAccessToken(apiKey, apiSecret string, grants claimGrants, ttl time.Duration, now time.Time) (string, error) {
	payload, err := json.Marshal(tokenClaims{
		Issuer:      apiKey,
		Subject:     grants.Identity,
		NotBefore:   now.Unix(),
		ExpiresAt:   now.Add(ttl).Unix(),
		claimGrants: grants,
	})
	if err != nil {
		return "", fmt.Errorf("livekit: marshal token claims: %w", err)
	}
	signingInput := jwtHeader + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, []byte(apiSecret))
	mac.Write([]byte(signingInput)) // hash.Hash.Write never returns an error
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
