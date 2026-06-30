.PHONY: dev-frontend dev-backend docker-up docker-down

dev-frontend:
	cd frontend && npm run dev

dev-backend:
	cd backend && go run cmd/main.go

docker-up:
	docker compose up -d

docker-down:
	docker compose down
