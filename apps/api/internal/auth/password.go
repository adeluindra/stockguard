// Package auth contains security primitives used by StockGuard authentication.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	// OWASP's minimum Argon2id configuration: 19 MiB, 2 iterations, 1 lane.
	memoryKiB   uint32 = 19 * 1024
	iterations  uint32 = 2
	parallelism uint8  = 1
	saltLength         = 16
	hashLength  uint32 = 32

	// This is a resource bound, not the user-facing password policy.
	maxPasswordBytes = 1024
)

var (
	ErrInvalidPassword = errors.New("password must be nonempty and at most 1024 bytes")
	ErrInvalidHash     = errors.New("stored password hash has an unsupported or invalid format")
)

// HashPassword returns a self-describing Argon2id password hash.
// A new random salt is generated on every invocation.
// Never log the plaintext password or include it in an HTTP response.
func HashPassword(password string) (string, error) {
	if len(password) == 0 || len(password) > maxPasswordBytes {
		return "", ErrInvalidPassword
	}

	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	derivedKey := argon2.IDKey(
		[]byte(password), salt, iterations, memoryKiB, parallelism, hashLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, memoryKiB, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(derivedKey),
	), nil
}

// VerifyPassword verifies a candidate password without revealing whether it matched
// through a timing-dependent byte-by-byte comparison.
//
// Only the current, bounded Argon2id parameters are accepted. Do not run
// attacker-controlled memory/CPU parameters obtained from arbitrary hash strings.
// For invalid stored hashes it returns ErrInvalidHash; callers must not expose
// this distinction in a public login response.
func VerifyPassword(password, encodedHash string) (bool, error) {
	if len(password) == 0 || len(password) > maxPasswordBytes {
		return false, nil
	}

	parts := strings.Split(encodedHash, "$")
	expectedParams := fmt.Sprintf("m=%d,t=%d,p=%d", memoryKiB, iterations, parallelism)
	expectedVersion := fmt.Sprintf("v=%d", argon2.Version)

	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" ||
		parts[2] != expectedVersion || parts[3] != expectedParams {
		return false, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != saltLength {
		return false, ErrInvalidHash
	}

	expectedKey, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expectedKey) != int(hashLength) {
		return false, ErrInvalidHash
	}

	candidateKey := argon2.IDKey(
		[]byte(password), salt, iterations, memoryKiB, parallelism, hashLength,
	)
	return subtle.ConstantTimeCompare(candidateKey, expectedKey) == 1, nil
}
