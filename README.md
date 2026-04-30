## face_control

Go backend that registers users on HASdk face-recognition devices over TCP (default port 9527).

### Quickstart

```bash
cp .env.example .env
make db-up          # starts postgres + minio in docker
make migrate-up     # creates tables, seeds test device 192.0.0.22
make run            # starts http server on :8080
```

Postgres on `:5432`, MinIO API on `:9000`, MinIO console on `:9001` (login `minioadmin`/`minioadmin`). The `face-photos` bucket is created automatically on first run.

The seed device row exists for local testing — its credentials default to `admin/admin`. Override with `POST /devices` for real hardware.

### HASdk client

By default the server runs with a `NoopClient` that logs registrations and returns success. The real C/C++ SDK lives behind a build tag:

```bash
go build -tags hasdk ./cmd/server
```

Building with `-tags hasdk` requires libHASdk headers and `.so` under `third_party/hasdk/` and a face model directory (`HASDK_MODEL_DIR`). The cgo bindings themselves are stubbed in `internal/hasdk/cgo_client.go` — finish them when the SDK files arrive.

### Endpoints

| Method | Path | Body |
| --- | --- | --- |
| `POST` | `/devices` | json `{name, ip, port?, username?, password?, mac?}` |
| `GET` | `/devices` | — |
| `GET` | `/devices/{id}` | — |
| `POST` | `/users` | multipart `full_name`, `photo` |
| `POST` | `/devices/{device_id}/registrations` | json `{user_id}` |
| `DELETE` | `/devices/{device_id}/registrations/{user_id}` | — |

### Layout

```
cmd/server                main.go composes everything
internal/api              chi handlers
internal/config           env loader
internal/device           device repo (sqlx)
internal/user             user repo
internal/registration     registration repo + service (orchestrates SDK + DB)
internal/hasdk            Client interface, NoopClient, cgo skeleton
internal/storage          PhotoStore interface + MinIO impl
migrations                golang-migrate SQL files
```
