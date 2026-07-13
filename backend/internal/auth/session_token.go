package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	// 32 bytes random => 43-ish chars base64url (no padding)
	SessionTokenBytes = 32
)

// NewSessionToken generates a new random session token: SessionTokenBytes of
// CSPRNG output, base64url-encoded without padding. The returned string is
// given to the client; only its SHA-256 hash (see HashSessionToken) is ever
// persisted server-side.
func NewSessionToken() (string, error) {
	b := make([]byte, SessionTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("authcore: read session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashSessionToken returns the SHA-256 hash of token (after trimming
// surrounding whitespace) as a 32-byte slice, suitable for storage and
// lookup. It returns an error if token is empty.
func HashSessionToken(token string) ([]byte, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("authcore: empty token")
	}
	sum := sha256.Sum256([]byte(token))
	// copy to slice so callers don't deal with array type
	out := make([]byte, 32)
	copy(out, sum[:])
	return out, nil
}
