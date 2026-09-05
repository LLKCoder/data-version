.PHONY: dev-frontend dev-backend dev-build compose-up compose-down

dev-frontend:
	cd frontend && npm run dev

dev-backend:
	cd backend && go run ./cmd/server

dev-build:
	cd frontend && npm run build
	cd backend && go build ./...

compose-up:
	docker compose up --build

compose-down:
	docker compose down

