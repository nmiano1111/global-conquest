package auth

import (
	"errors"
	"testing"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", DefaultPasswordParams())
	if err != nil {
		t.Fatalf("hash: %v", err)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !ok {
		t.Fatalf("expected ok")
	}

	ok, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("verify wrong: %v", err)
	}
	if ok {
		t.Fatalf("expected not ok")
	}
}

func TestHashPasswordTooShort(t *testing.T) {
	_, err := HashPassword("short", DefaultPasswordParams())
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Fatalf("expected ErrPasswordTooShort, got %v", err)
	}
}

func TestVerifyPasswordMalformedHash(t *testing.T) {
	ok, err := VerifyPassword("any-password", "not-a-valid-argon2-hash")
	if ok {
		t.Fatalf("expected not ok")
	}
	if !errors.Is(err, ErrHashFormatInvalid) {
		t.Fatalf("expected ErrHashFormatInvalid, got %v", err)
	}
}

func TestValidateUsername(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "valid simple", in: "alice123", want: "alice123"},
		{name: "valid separators", in: "player-one_2", want: "player-one_2"},
		{name: "trim spaces", in: "  bob  ", want: "bob"},
		{name: "too short", in: "ab", wantErr: true},
		{name: "too long", in: "this_name_is_way_too_long_123", wantErr: true},
		{name: "invalid character", in: "bad.name", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := ValidateUsername(tc.in)
			if tc.wantErr {
				if !errors.Is(err, ErrUsernameInvalid) {
					t.Fatalf("expected ErrUsernameInvalid, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}
