CREATE TABLE devices (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    ip          TEXT NOT NULL,
    port        INT  NOT NULL DEFAULT 9527,
    username    TEXT NOT NULL DEFAULT 'admin',
    password    TEXT NOT NULL DEFAULT 'admin',
    mac         TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (ip, port)
);

CREATE TABLE users (
    id          UUID PRIMARY KEY,
    full_name   TEXT NOT NULL,
    photo_key   TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TYPE registration_status AS ENUM ('pending', 'registered', 'failed', 'deleted');

CREATE TABLE device_registrations (
    device_id        UUID NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    user_id          UUID NOT NULL REFERENCES users(id)   ON DELETE CASCADE,
    face_id          TEXT NOT NULL,
    status           registration_status NOT NULL DEFAULT 'pending',
    error_message    TEXT,
    registered_at    TIMESTAMPTZ,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (device_id, user_id),
    UNIQUE (device_id, face_id)
);

CREATE INDEX idx_device_registrations_user ON device_registrations(user_id);
