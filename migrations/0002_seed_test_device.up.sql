INSERT INTO devices (id, name, ip, port, username, password)
VALUES ('11111111-1111-1111-1111-111111111111', 'test-device', '192.0.0.22', 8000, 'admin', 'admin')
ON CONFLICT DO NOTHING;
