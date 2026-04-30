package user

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type User struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	FullName  string    `db:"full_name"  json:"full_name"`
	PhotoKey  string    `db:"photo_key"  json:"photo_key"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}

type Repo struct{ db *sqlx.DB }

func NewRepo(db *sqlx.DB) *Repo { return &Repo{db: db} }

func (r *Repo) Create(ctx context.Context, u *User) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	const q = `INSERT INTO users (id, full_name, photo_key)
	           VALUES ($1,$2,$3) RETURNING created_at`
	return r.db.QueryRowxContext(ctx, q, u.ID, u.FullName, u.PhotoKey).Scan(&u.CreatedAt)
}

func (r *Repo) Get(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	const q = `SELECT * FROM users WHERE id = $1`
	if err := r.db.GetContext(ctx, &u, q, id); err != nil {
		return nil, err
	}
	return &u, nil
}
