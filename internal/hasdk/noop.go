package hasdk

import (
	"context"
	"log/slog"
)

// NoopClient satisfies Client without touching the real SDK. Useful for local
// development before the libHASdk shared object and face model files are
// available. Every call logs and returns nil.
type NoopClient struct {
	Log *slog.Logger
}

func NewNoopClient(log *slog.Logger) *NoopClient {
	if log == nil {
		log = slog.Default()
	}
	return &NoopClient{Log: log}
}

func (c *NoopClient) Register(_ context.Context, req RegisterRequest) error {
	c.Log.Info("hasdk.noop register",
		"device_id", req.Device.ID,
		"host", req.Device.Host,
		"port", req.Device.Port,
		"face_id", req.FaceID,
		"face_name", req.FaceName,
		"role", req.Role,
		"jpeg_bytes", len(req.JPEG),
	)
	return nil
}

func (c *NoopClient) Delete(_ context.Context, dev Device, faceID string) error {
	c.Log.Info("hasdk.noop delete",
		"device_id", dev.ID,
		"host", dev.Host,
		"face_id", faceID,
	)
	return nil
}

func (c *NoopClient) Ping(_ context.Context, dev Device) (string, error) {
	c.Log.Info("hasdk.noop ping", "device_id", dev.ID, "host", dev.Host)
	return "noop-device", nil
}

func (c *NoopClient) Close() error { return nil }
