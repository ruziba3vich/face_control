.PHONY: db-up db-down migrate-up migrate-down run tidy

DB_URL ?= postgres://face_control:face_control@localhost:5432/face_control?sslmode=disable

db-up:
	docker compose -f docker/docker-compose.yml up -d

db-down:
	docker compose -f docker/docker-compose.yml down

migrate-up:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
	    -path migrations -database "$(DB_URL)" up

migrate-down:
	go run -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest \
	    -path migrations -database "$(DB_URL)" down 1

run:
	go run ./cmd/server

tidy:
	go mod tidy
