package hasdk

import "context"

// Device addresses one HASdk camera. The SDK has no native device-id concept,
// so the backend's UUID is mapped to (Host, Port, Username, Password) here.
type Device struct {
	ID       string
	Host     string
	Port     uint16
	Username string
	Password string
}

// FaceRole maps to FaceFlags.role: 0=normal, 1=whitelisted, 2=blacklisted.
type FaceRole int

const (
	RoleNormal      FaceRole = 0
	RoleWhitelisted FaceRole = 1
	RoleBlacklisted FaceRole = 2
)

// RegisterRequest is everything HA_AddJpgFaces needs for one face.
type RegisterRequest struct {
	Device   Device
	FaceID   string // ≤ 20 bytes, see hasdk.NewFaceID
	FaceName string // ≤ 16 bytes
	Role     FaceRole
	JPEG     []byte // ≤ 10MB; jpg/bmp/png all accepted by the SDK
}

// Client is the abstraction the rest of the app talks to. Implementations:
//   - NoopClient: logs and pretends to succeed (default until the SDK is wired)
//   - cgoClient (TODO): links libHASdk and calls HA_AddJpgFaces et al.
type Client interface {
	Register(ctx context.Context, req RegisterRequest) error
	Delete(ctx context.Context, dev Device, faceID string) error
	Close() error
}
