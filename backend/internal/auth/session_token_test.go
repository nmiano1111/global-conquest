package auth

import (
	"encoding/base64"
	"testing"
)

func TestSessionTokenHash(t *testing.T) {
	tok, err := NewSessionToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}

	h1, err := HashSessionToken(tok)
	if err != nil {
		t.Fatalf("hash1: %v", err)
	}
	h2, err := HashSessionToken(tok)
	if err != nil {
		t.Fatalf("hash2: %v", err)
	}

	if string(h1) != string(h2) {
		t.Fatalf("expected deterministic hash")
	}
}

func TestNewSessionTokenFormat(t *testing.T) {
	tok, err := NewSessionToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	if len(tok) < 40 {
		t.Fatalf("expected long token, got len=%d", len(tok))
	}
	if _, err := base64.RawURLEncoding.DecodeString(tok); err != nil {
		t.Fatalf("token is not base64url: %v", err)
	}
}

func TestHashSessionTokenEmpty(t *testing.T) {
	if _, err := HashSessionToken(""); err == nil {
		t.Fatalf("expected error for empty token")
	}
}
