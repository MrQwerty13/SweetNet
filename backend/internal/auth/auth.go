// Package auth contains credential primitives, never HTTP or database policies.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/alexedwards/argon2id"
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)
var params = &argon2id.Params{Memory: 19 * 1024, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32}

func Username(value string) (string, bool) {
	value = strings.ToLower(value)
	return value, usernamePattern.MatchString(value)
}
func ValidName(value string) bool {
	return utf8.ValidString(value) && len(strings.TrimSpace(value)) > 0 && utf8.RuneCountInString(value) <= 80
}
func ValidPassword(value string) bool {
	n := utf8.RuneCountInString(value)
	return utf8.ValidString(value) && n >= 12 && n <= 128
}
func HashPassword(value string) (string, error) { return argon2id.CreateHash(value, params) }
func CheckPassword(value, hash string) bool {
	ok, err := argon2id.ComparePasswordAndHash(value, hash)
	return err == nil && ok
}
func Secret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func HashSecret(value string) string {
	h := sha256.Sum256([]byte(value))
	return hex.EncodeToString(h[:])
}
