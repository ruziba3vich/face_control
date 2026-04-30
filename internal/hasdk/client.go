package hasdk

import "context"

// Device addresses one FaceGate camera. The vendor's HTTP API has no
// connection-level concept of a device id; we identify each device by
// (Host, Port, Username, Password) and remember its serial number out of band.
type Device struct {
	ID       string // backend's UUID; passed through for logging/telemetry only
	Host     string
	Port     uint16
	Username string
	Password string
}

// FaceRole maps to the device's FaceFlags.role.
//   1 = whitelisted (matched person grants access),
//   2 = blacklisted (matched person triggers alarm).
// 0 is reserved by the device for "ordinary personnel" semantics used in
// query/delete flags but is rejected at registration time, so we default to 1.
type FaceRole int

const (
	RoleNormal      FaceRole = 0
	RoleWhitelisted FaceRole = 1
	RoleBlacklisted FaceRole = 2
)

// RegisterRequest is everything one "add person jpg" call needs.
type RegisterRequest struct {
	Device   Device
	FaceID   string // ≤ 19 bytes; see hasdk.NewFaceID for the surrogate strategy
	FaceName string // ≤ 15 bytes
	Role     FaceRole
	JPEG     []byte // ≤ 10MB; vendor recommends ≤4MB for speed
}

// Client is the abstraction the rest of the app talks to.
//
// Implementations in this package:
//   - HTTPClient (default): POSTs JSON to the device's HTTP API on port 8000.
//   - NoopClient: logs and pretends to succeed; useful for dev without hardware.
type Client interface {
	Register(ctx context.Context, req RegisterRequest) error
	Delete(ctx context.Context, dev Device, faceID string) error
	// Ping verifies the device is reachable and returns its serial number.
	// Implementations should be fast (a single small request).
	Ping(ctx context.Context, dev Device) (deviceSN string, err error)
	Close() error
}
