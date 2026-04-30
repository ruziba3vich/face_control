package registration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"udevs/face_control/internal/device"
	"udevs/face_control/internal/hasdk"
	"udevs/face_control/internal/storage"
	"udevs/face_control/internal/user"
)

type Status string

const (
	StatusPending    Status = "pending"
	StatusRegistered Status = "registered"
	StatusFailed     Status = "failed"
	StatusDeleted    Status = "deleted"
)

type Registration struct {
	DeviceID     uuid.UUID    `db:"device_id"     json:"device_id"`
	UserID       uuid.UUID    `db:"user_id"       json:"user_id"`
	FaceID       string       `db:"face_id"       json:"face_id"`
	Status       Status       `db:"status"        json:"status"`
	ErrorMessage *string      `db:"error_message" json:"error_message,omitempty"`
	RegisteredAt sql.NullTime `db:"registered_at" json:"registered_at,omitempty"`
	CreatedAt    time.Time    `db:"created_at"    json:"created_at"`
	UpdatedAt    time.Time    `db:"updated_at"    json:"updated_at"`
}

type Repo struct{ db *sqlx.DB }

func NewRepo(db *sqlx.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Upsert(ctx context.Context, reg *Registration) error {
	const q = `
INSERT INTO device_registrations (device_id, user_id, face_id, status, error_message, registered_at)
VALUES ($1,$2,$3,$4,$5,$6)
ON CONFLICT (device_id, user_id) DO UPDATE
SET status        = EXCLUDED.status,
    error_message = EXCLUDED.error_message,
    registered_at = EXCLUDED.registered_at,
    updated_at    = now()
RETURNING created_at, updated_at`
	return r.db.QueryRowxContext(ctx, q,
		reg.DeviceID, reg.UserID, reg.FaceID, reg.Status, reg.ErrorMessage, reg.RegisteredAt,
	).Scan(&reg.CreatedAt, &reg.UpdatedAt)
}

func (r *Repo) Delete(ctx context.Context, deviceID, userID uuid.UUID) error {
	_, err := r.db.ExecContext(ctx,
		`DELETE FROM device_registrations WHERE device_id = $1 AND user_id = $2`,
		deviceID, userID)
	return err
}

func (r *Repo) Get(ctx context.Context, deviceID, userID uuid.UUID) (*Registration, error) {
	var reg Registration
	const q = `SELECT * FROM device_registrations WHERE device_id = $1 AND user_id = $2`
	if err := r.db.GetContext(ctx, &reg, q, deviceID, userID); err != nil {
		return nil, err
	}
	return &reg, nil
}

func (r *Repo) ListByDevice(ctx context.Context, deviceID uuid.UUID) ([]Registration, error) {
	var out []Registration
	const q = `SELECT * FROM device_registrations WHERE device_id = $1 ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &out, q, deviceID); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repo) ListByUser(ctx context.Context, userID uuid.UUID) ([]Registration, error) {
	var out []Registration
	const q = `SELECT * FROM device_registrations WHERE user_id = $1 ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &out, q, userID); err != nil {
		return nil, err
	}
	return out, nil
}

// Service ties the HASdk client, the three repos, and the user's photo file
// together. RegisterUser is the main flow: load device + user, derive faceID,
// read photo bytes, push to device, persist outcome.
type Service struct {
	Devices       *device.Repo
	Users         *user.Repo
	Registrations *Repo
	Photos        storage.PhotoStore
	HASdk         hasdk.Client
}

var (
	ErrDeviceNotFound = errors.New("device not found")
	ErrUserNotFound   = errors.New("user not found")
)

func (s *Service) RegisterUser(ctx context.Context, deviceID, userID uuid.UUID) (*Registration, error) {
	d, err := s.Devices.Get(ctx, deviceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDeviceNotFound
		}
		return nil, fmt.Errorf("load device: %w", err)
	}
	u, err := s.Users.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("load user: %w", err)
	}

	jpeg, err := s.Photos.Get(ctx, u.PhotoKey)
	if err != nil {
		return nil, fmt.Errorf("read photo: %w", err)
	}

	faceID := hasdk.NewFaceID(u.ID.String())
	reg := &Registration{
		DeviceID: d.ID,
		UserID:   u.ID,
		FaceID:   faceID,
		Status:   StatusPending,
	}
	if err := s.Registrations.Upsert(ctx, reg); err != nil {
		return nil, fmt.Errorf("persist pending: %w", err)
	}

	sdkErr := s.HASdk.Register(ctx, hasdk.RegisterRequest{
		Device:   toSdkDevice(d),
		FaceID:   faceID,
		FaceName: truncate(u.FullName, 15), // device caps name at 15 bytes (HTTP_En.pdf §2.1.3)
		Role:     hasdk.RoleWhitelisted,    // device rejects role=0 at registration; default whitelist
		JPEG:     jpeg,
	})
	if sdkErr != nil {
		msg := sdkErr.Error()
		reg.Status = StatusFailed
		reg.ErrorMessage = &msg
		_ = s.Registrations.Upsert(ctx, reg)
		return reg, sdkErr
	}

	now := time.Now()
	reg.Status = StatusRegistered
	reg.ErrorMessage = nil
	reg.RegisteredAt = sql.NullTime{Time: now, Valid: true}
	if err := s.Registrations.Upsert(ctx, reg); err != nil {
		return reg, fmt.Errorf("persist success: %w", err)
	}
	return reg, nil
}

func (s *Service) DeleteRegistration(ctx context.Context, deviceID, userID uuid.UUID) error {
	d, err := s.Devices.Get(ctx, deviceID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrDeviceNotFound
		}
		return err
	}
	reg, err := s.Registrations.Get(ctx, deviceID, userID)
	if err != nil {
		return err
	}
	if err := s.HASdk.Delete(ctx, toSdkDevice(d), reg.FaceID); err != nil {
		return err
	}
	return s.Registrations.Delete(ctx, deviceID, userID)
}

func toSdkDevice(d *device.Device) hasdk.Device {
	return hasdk.Device{
		ID:       d.ID.String(),
		Host:     d.IP,
		Port:     uint16(d.Port),
		Username: d.Username,
		Password: d.Password,
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
