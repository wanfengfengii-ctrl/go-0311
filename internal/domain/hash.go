package domain

import (
	"crypto/sha256"
	"encoding/hex"
)

// CanonicalHash returns the SHA-256 hex digest of a canonical payload. It is
// used as the stable payload summary recorded on every evidence event and
// idempotency record so that retries with identical content match and tampering
// is detectable after restart.
func CanonicalHash(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(sum[:])
}
