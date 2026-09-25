package authn

import (
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2Params are the argon2id cost parameters.
type Argon2Params struct {
	Time    uint32
	Memory  uint32 // KiB
	Threads uint8
	KeyLen  uint32
}

// Argon2 hashes secrets (OTP codes) with argon2id in PHC string format.
type Argon2 struct {
	p    Argon2Params
	rand io.Reader
}

// NewArgon2 returns a hasher drawing salts from rand.
func NewArgon2(p Argon2Params, rand io.Reader) *Argon2 { return &Argon2{p: p, rand: rand} }

var b64 = base64.RawStdEncoding

// Hash returns $argon2id$v=19$m=..,t=..,p=..$salt$key.
func (a *Argon2) Hash(secret string) (string, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(a.rand, salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(secret), salt, a.p.Time, a.p.Memory, a.p.Threads, a.p.KeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, a.p.Memory, a.p.Time, a.p.Threads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// Verify checks secret against an encoded hash in constant time, using the
// parameters recorded in the hash (so cost changes never break old hashes).
func (a *Argon2) Verify(secret, encoded string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	salt, err1 := b64.DecodeString(parts[4])
	want, err2 := b64.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(secret), salt, t, m, p, uint32(len(want))) //nolint:gosec // len(want) is bounded by the stored hash
	return subtle.ConstantTimeCompare(got, want) == 1
}
