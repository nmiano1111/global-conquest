package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

// PasswordParams tunes the Argon2id key-derivation parameters used to hash
// and verify passwords.
type PasswordParams struct {
	Memory uint32 // KiB
	// Iterations is the number of Argon2id passes over memory.
	Iterations uint32
	// Parallelism is the number of parallel Argon2id lanes (threads).
	Parallelism uint8
	// SaltLen is the length, in bytes, of the random salt generated for each hash.
	SaltLen uint32
	// KeyLen is the length, in bytes, of the derived key (hash output).
	KeyLen uint32
}

// DefaultPasswordParams returns a baseline set of Argon2id parameters
// (64 MiB memory, 3 iterations, parallelism 2, 16-byte salt, 32-byte key)
// suitable for a hobby-scale server. Callers wanting stronger guarantees
// should tune Memory upward.
func DefaultPasswordParams() PasswordParams {
	// Good baseline for a hobby app on a typical server.
	// Tune later if you want (memory is the big lever).
	return PasswordParams{
		Memory:      64 * 1024, // 64 MiB
		Iterations:  3,
		Parallelism: 2,
		SaltLen:     16,
		KeyLen:      32,
	}
}

// HashPassword hashes plaintext using Argon2id with the given parameters and
// a freshly generated random salt, returning the result encoded as
// "$argon2id$v=<version>$m=<memory>,t=<iterations>,p=<parallelism>$<salt>$<hash>".
// It returns ErrPasswordTooShort if the trimmed plaintext is under 8 characters.
func HashPassword(plaintext string, p PasswordParams) (string, error) {
	plaintext = strings.TrimSpace(plaintext)
	if len(plaintext) < 8 {
		return "", ErrPasswordTooShort
	}

	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("authcore: read salt: %w", err)
	}

	hash := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLen)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encoded := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism, b64Salt, b64Hash)

	return encoded, nil
}

// VerifyPassword reports whether plaintext matches the Argon2id hash encoded
// in encodedHash, using the parameters embedded in encodedHash and a
// constant-time comparison. It returns (false, nil) rather than an error for
// empty inputs, to avoid leaking which side was invalid, and returns a
// non-nil error only when encodedHash is malformed.
func VerifyPassword(plaintext, encodedHash string) (bool, error) {
	plaintext = strings.TrimSpace(plaintext)
	encodedHash = strings.TrimSpace(encodedHash)
	if plaintext == "" || encodedHash == "" {
		// Don't leak which part is wrong.
		return false, nil
	}

	p, salt, expected, err := parseEncodedArgon2id(encodedHash)
	if err != nil {
		// Treat malformed hash as non-match (but return error so you can log internally).
		return false, err
	}

	got := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, uint32(len(expected)))

	// constant-time compare
	if subtle.ConstantTimeCompare(got, expected) == 1 {
		return true, nil
	}
	return false, nil
}

// ValidateUsername trims username and validates that it is between 3 and 24
// characters and contains only letters, digits, underscores, and hyphens,
// returning ErrUsernameInvalid otherwise.
func ValidateUsername(username string) (string, error) {
	u := strings.TrimSpace(username)
	if len(u) < 3 || len(u) > 24 {
		return "", ErrUsernameInvalid
	}
	for _, r := range u {
		if r == '_' || r == '-' {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		return "", ErrUsernameInvalid
	}
	return u, nil
}

func parseEncodedArgon2id(encoded string) (PasswordParams, []byte, []byte, error) {
	// $argon2id$v=19$m=65536,t=3,p=2$<salt>$<hash>
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}

	// version
	if !strings.HasPrefix(parts[2], "v=") {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}
	ver, err := strconv.Atoi(strings.TrimPrefix(parts[2], "v="))
	if err != nil || ver != argon2.Version {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}

	// params
	if !strings.HasPrefix(parts[3], "m=") {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}
	paramParts := strings.Split(parts[3], ",")
	if len(paramParts) != 3 {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}

	memStr := strings.TrimPrefix(paramParts[0], "m=")
	itStr := strings.TrimPrefix(paramParts[1], "t=")
	parStr := strings.TrimPrefix(paramParts[2], "p=")

	mem64, err := strconv.ParseUint(memStr, 10, 32)
	if err != nil {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}
	it64, err := strconv.ParseUint(itStr, 10, 32)
	if err != nil {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}
	par64, err := strconv.ParseUint(parStr, 10, 8)
	if err != nil {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) < 8 {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(hash) < 16 {
		return PasswordParams{}, nil, nil, ErrHashFormatInvalid
	}

	pp := PasswordParams{
		Memory:      uint32(mem64),
		Iterations:  uint32(it64),
		Parallelism: uint8(par64),
		SaltLen:     uint32(len(salt)),
		KeyLen:      uint32(len(hash)),
	}

	return pp, salt, hash, nil
}
