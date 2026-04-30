package device

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Device struct {
	ID        uuid.UUID `db:"id"        json:"id"`
	Name      string    `db:"name"      json:"name"`
	IP        string    `db:"ip"        json:"ip"`
	Port      int       `db:"port"      json:"port"`
	Username  string    `db:"username"  json:"username"`
	Password  string    `db:"password"  json:"-"`
	MAC       *string   `db:"mac"       json:"mac,omitempty"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Repo struct{ db *sqlx.DB }

func NewRepo(db *sqlx.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(ctx context.Context, d *Device) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	if d.Port == 0 {
		d.Port = 9527
	}
	if d.Username == "" {
		d.Username = "admin"
	}
	if d.Password == "" {
		d.Password = "admin"
	}
	const q = `INSERT INTO devices (id, name, ip, port, username, password, mac)
	           VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING created_at`
	return r.db.QueryRowxContext(ctx, q, d.ID, d.Name, d.IP, d.Port, d.Username, d.Password, d.MAC).Scan(&d.CreatedAt)
}

func (r *Repo) Get(ctx context.Context, id uuid.UUID) (*Device, error) {
	var d Device
	const q = `SELECT * FROM devices WHERE id = $1`
	if err := r.db.GetContext(ctx, &d, q, id); err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *Repo) List(ctx context.Context) ([]Device, error) {
	var out []Device
	const q = `SELECT * FROM devices ORDER BY created_at DESC`
	if err := r.db.SelectContext(ctx, &out, q); err != nil {
		return nil, err
	}
	return out, nil
}
