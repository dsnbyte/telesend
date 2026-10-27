// Package secret hashes and verifies API keys and owner passwords.
//
// Hashes use PBKDF2-HMAC-SHA256 from the standard library and are stored as
// "$pbkdf2-sha256$i=<iterations>$<salt>$<key>". For dotenv files, where "$"
// is awkward, the hash may be base64url-encoded (see Encode).
package secret

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	iterations = 600_000
	keyLength  = 32
	prefix     = "$pbkdf2-sha256$"
)

var b64 = base64.RawStdEncoding

// Hash derives a salted hash of secret.
func Hash(secret string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key, err := pbkdf2.Key(sha256.New, secret, salt, iterations, keyLength)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%si=%d$%s$%s", prefix, iterations, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

type parsedHash struct {
	iterations int
	salt, key  []byte
}

func parse(hash string) (parsedHash, error) {
	rest, ok := strings.CutPrefix(hash, prefix)
	if !ok {
		return parsedHash{}, errors.New("unsupported hash format")
	}
	parts := strings.Split(rest, "$")
	if len(parts) != 3 || !strings.HasPrefix(parts[0], "i=") {
		return parsedHash{}, errors.New("malformed hash")
	}
	n, err := strconv.Atoi(parts[0][2:])
	if err != nil || n < 1 || n > 10_000_000 {
		return parsedHash{}, errors.New("malformed hash")
	}
	salt, err1 := b64.DecodeString(parts[1])
	key, err2 := b64.DecodeString(parts[2])
	if err1 != nil || err2 != nil || len(salt) == 0 || len(key) == 0 {
		return parsedHash{}, errors.New("malformed hash")
	}
	return parsedHash{n, salt, key}, nil
}

// Verify reports whether secret matches hash.
func Verify(secret, hash string) bool {
	p, err := parse(hash)
	if err != nil {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, secret, p.salt, p.iterations, len(p.key))
	return err == nil && subtle.ConstantTimeCompare(key, p.key) == 1
}

// Encode makes a hash safe to store in a dotenv file.
func Encode(hash string) string { return base64.RawURLEncoding.EncodeToString([]byte(hash)) }

// Parse accepts a raw or Encode'd hash and returns the validated raw hash.
func Parse(value string) (string, error) {
	hash := value
	if !strings.HasPrefix(value, "$") {
		raw, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			return "", errors.New("not a valid hash")
		}
		hash = string(raw)
	}
	if _, err := parse(hash); err != nil {
		return "", err
	}
	return hash, nil
}

// Verifier checks presented secrets against a configured hash or plaintext.
// After one successful slow verification it remembers a SHA-256 digest of the
// accepted secret in memory so later requests are cheap.
type Verifier struct {
	hash    string
	plain   [32]byte
	hasText bool
	plainIn string
	cached  atomic.Pointer[[32]byte]
}

// NewVerifier prefers hashValue over plaintext. It returns (nil, nil) when
// neither is configured and an error when hashValue is invalid.
func NewVerifier(hashValue, plaintext string) (*Verifier, error) {
	if hashValue = strings.TrimSpace(hashValue); hashValue != "" {
		hash, err := Parse(hashValue)
		if err != nil {
			return nil, err
		}
		return &Verifier{hash: hash}, nil
	}
	if plaintext = strings.TrimSpace(plaintext); plaintext != "" {
		return &Verifier{plain: sha256.Sum256([]byte(plaintext)), hasText: true, plainIn: plaintext}, nil
	}
	return nil, nil
}

// Verify reports whether provided is the configured secret.
func (v *Verifier) Verify(provided string) bool {
	sum := sha256.Sum256([]byte(provided))
	if v.hasText {
		return subtle.ConstantTimeCompare(sum[:], v.plain[:]) == 1
	}
	if c := v.cached.Load(); c != nil && subtle.ConstantTimeCompare(sum[:], c[:]) == 1 {
		return true
	}
	if !Verify(provided, v.hash) {
		return false
	}
	v.cached.Store(&sum)
	return true
}

// Secrets lists values that must never appear in output.
func (v *Verifier) Secrets() []string {
	if v.hasText {
		return []string{v.plainIn}
	}
	return []string{v.hash}
}
