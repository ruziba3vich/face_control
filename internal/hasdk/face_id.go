package hasdk

import (
	"crypto/sha256"
	"encoding/base32"
	"strings"
)

// MaxFaceIDLen is the SDK's faceID buffer size (FaceFlags.faceID[20]).
const MaxFaceIDLen = 19 // leave a byte for the C null terminator

// NewFaceID derives a stable, ≤19-char ASCII id from a UUID. Base32 (no padding,
// lowercase) of a SHA-256 prefix — collision-resistant enough for one device's
// face DB, and fixed-length so device_registrations.face_id is predictable.
func NewFaceID(userUUID string) string {
	sum := sha256.Sum256([]byte(userUUID))
	enc := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
	return enc[:MaxFaceIDLen]
}
