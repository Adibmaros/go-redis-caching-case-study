.PHONY: redis-up redis-down migrate seed run redis-cli build test

# Jalankan Redis via Podman Compose (atau fallback docker compose)
redis-up:
	@podman compose up -d 2>/dev/null || podman-compose up -d 2>/dev/null || docker compose up -d

# Matikan Redis
redis-down:
	@podman compose down 2>/dev/null || podman-compose down 2>/dev/null || docker compose down

# Masuk ke Redis CLI
redis-cli:
	@podman exec -it redis-case-study redis-cli 2>/dev/null || docker exec -it redis-case-study redis-cli

# Eksekusi migrasi skema tabel ke MySQL
migrate:
	go run ./cmd/seeder -migrate-only

# Jalankan seeder data dummy
seed:
	go run ./cmd/seeder

# Jalankan server API
run:
	go run ./cmd/api

# Build binary
build:
	go build -o bin/api.exe ./cmd/api
	go build -o bin/seeder.exe ./cmd/seeder

test:
	go test -v ./...
