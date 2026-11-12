.PHONY: up down logs test fmt tidy terraform-init terraform-plan

up:
	docker compose up --build -d

down:
	docker compose down

logs:
	docker compose logs -f frontend api outbox worker

test:
	go test ./...

fmt:
	gofmt -w $$(find cmd internal -name '*.go')

tidy:
	go mod tidy

frontend-check:
	cd frontend && npm run typecheck && npm run build

terraform-init:
	terraform -chdir=infrastructure/terraform init

terraform-plan:
	terraform -chdir=infrastructure/terraform plan
