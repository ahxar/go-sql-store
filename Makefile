.PHONY: help docker-up docker-down migrate-up migrate-down run test clean

help:
	@echo "Available targets:"
	@echo "  docker-up      - Start PostgreSQL container"
	@echo "  docker-down    - Stop PostgreSQL container"
	@echo "  migrate-up     - Run database migrations"
	@echo "  migrate-down   - Rollback database migrations"
	@echo "  run            - Run the application"
	@echo "  test           - Run integration tests"
	@echo "  clean          - Remove binaries and temporary files"

docker-up:
	docker-compose up -d
	@echo "Waiting for PostgreSQL to be ready..."
	@sleep 3

docker-down:
	docker-compose down

migrate-up:
	go run scripts/run_migrations.go up

migrate-down:
	go run scripts/run_migrations.go down

run:
	go run cmd/api/main.go

test:
	go test -v ./tests/integration/...

clean:
	rm -rf bin/
	go clean -testcache
