# OC Broker

OC Broker is a runnable Go reference implementation for asynchronous insurance claim processing. It registers claimants, accepts and queries claims, persists data in PostgreSQL, publishes commands through a transactional outbox, processes them from Amazon SQS, and emits claim result events.

The repository also includes a Next.js claimant portal that runs locally through Docker Compose and can be deployed independently to Vercel.

## Architecture

```text
HTTP client
    │
    ▼
Next.js portal (:3000)
    │ server-side API proxy
    ▼
claim-api ---> PostgreSQL (claim + outbox in one transaction)
                    |
                    v
              outbox publisher
                    |
                    v
             claim-commands SQS ---> claim-worker
                                           |
                                           +---> PostgreSQL (decision + inbox)
                                           |
                                           +---> outbox ---> claim-events SQS
```

SQS is provided locally by LocalStack. Standard queues use at-least-once delivery, so the worker records every processed event ID in `inbox_messages`. A dead-letter queue receives commands that still fail after five deliveries.

## Prerequisites

- Docker with Docker Compose
- Go 1.25+ only when running or testing outside Docker
- Node.js 22+ and npm only when running the frontend outside Docker
- Terraform 1.6+ only when provisioning queues independently

## Quick start

```sh
make up
docker compose ps
curl http://localhost:8080/health/ready
```

The first build downloads the Go, Node, PostgreSQL, and LocalStack dependencies. Once every service is healthy, open the claimant portal at [http://localhost:3000](http://localhost:3000).

| Service | Local address | Purpose |
|---|---|---|
| Next.js portal | `http://localhost:3000` | Browser interface and server-side API proxy |
| Go API | `http://localhost:8080` | Claimant and claim HTTP endpoints |
| PostgreSQL | Internal Docker network only | Business data, inbox, and outbox storage |
| LocalStack | `http://localhost:4566` | Local SQS implementation |

The portal supports the complete current workflow:

1. Register a claimant or look up an existing claimant ID.
2. Submit a claim with policy, incident, amount, and description details.
3. Watch a claim move from `PENDING` to `APPROVED` or `REVIEW_REQUIRED`.
4. Review the selected claimant's claim history.

The selected claimant ID is retained in browser storage for convenience. It is not an authentication mechanism.

### Run the frontend outside Docker

To run only the frontend outside Docker while the Go stack is already running:

```sh
cd frontend
cp .env.example .env.local
npm install
npm run dev
```

The root `.env.example` contains working defaults for the Go containers. `frontend/.env.example` points a standalone Next.js development server at the local Go API. Create ignored `.env` or `.env.local` files only when overriding those defaults.

### LocalStack startup troubleshooting

The LocalStack health check verifies that the command and event queues were created, rather than checking for one particular service-state word. LocalStack reports a service as `available` before its first request and `running` afterwards, so both states are healthy.

If `docker compose logs localstack` reports a missing or invalid license credential, current LocalStack releases require an Auth Token. Create a token in your LocalStack account and export it before starting Compose:

```sh
export LOCALSTACK_AUTH_TOKEN='ls-...'
docker compose up --build
```

Do not add a real LocalStack token to `.env.example` or commit it.

## Try the workflow

Register a claimant:

```sh
curl -i http://localhost:8080/v1/claimants \
  -H 'Content-Type: application/json' \
  -d '{"fullName":"Alex Morgan","email":"alex.morgan@example.com"}'
```

Copy the returned claimant `id`, then submit a claim:

```sh
curl -i http://localhost:8080/v1/claims \
  -H 'Content-Type: application/json' \
  -d '{
    "claimantId":"REPLACE_WITH_CLAIMANT_ID",
    "policyNumber":"OC-2026-001",
    "incidentDate":"2026-11-09",
    "amountCents":125000,
    "currency":"GBP",
    "description":"Water damage to the kitchen ceiling",
    "idempotencyKey":"demo-claim-0001"
  }'
```

The API returns `202 Accepted` and a `PENDING` claim. The outbox and worker normally change it to `APPROVED` within a few seconds. Claims above 1,000,000 minor currency units move to `REVIEW_REQUIRED`.

```sh
curl http://localhost:8080/v1/claims/REPLACE_WITH_CLAIM_ID
curl http://localhost:8080/v1/claimants/REPLACE_WITH_CLAIMANT_ID/claims
```

Submitting the same `idempotencyKey` again returns the original claim rather than creating a duplicate.

## API

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/health/live` | Process liveness |
| `GET` | `/health/ready` | Database readiness |
| `POST` | `/v1/claimants` | Register a claimant |
| `GET` | `/v1/claimants/{id}` | View a claimant account |
| `GET` | `/v1/claimants/{id}/claims` | View claim history |
| `POST` | `/v1/claims` | Submit a claim |
| `GET` | `/v1/claims/{id}` | View a claim and its processing status |

## Development commands

```sh
make test             # run Go unit tests
make fmt              # format Go sources
make frontend-check   # type-check and build the Next.js portal
make logs             # follow frontend, API, outbox, and worker logs
make down             # stop containers without deleting PostgreSQL data
```

To reset all local data, explicitly run `docker compose down --volumes`.

The frontend can also be checked directly:

```sh
cd frontend
npm install
npm run typecheck
npm run build
```

Commit the generated `frontend/package-lock.json` after the first successful `npm install` so local, CI, and Vercel builds resolve the same dependency graph.

## Project layout

```text
cmd/api/                 Go HTTP process
cmd/outbox/              Transactional outbox publisher
cmd/worker/              SQS claim-processing worker
internal/httpapi/        HTTP routes and request validation
internal/store/          PostgreSQL queries and transactions
internal/messaging/      SQS client configuration
db/migrations/           PostgreSQL schema
frontend/                Next.js claimant portal
infrastructure/terraform SQS infrastructure definitions
scripts/                 LocalStack queue initialization
```

## Configuration

The most important runtime variables are:

| Variable | Used by | Purpose |
|---|---|---|
| `DATABASE_URL` | Go processes | PostgreSQL connection string |
| `AWS_ENDPOINT_URL` | Go processes | LocalStack endpoint locally; empty when using AWS |
| `CLAIM_COMMANDS_QUEUE_URL` | Outbox and worker | Input command queue |
| `CLAIM_EVENTS_QUEUE_URL` | Outbox | Processed-claim event queue |
| `WORKER_CONCURRENCY` | Worker | Maximum concurrent message handlers |
| `OC_API_URL` | Next.js server | Upstream Go API used by `/api/oc/*` |

## Terraform

Docker Compose creates queues through a LocalStack readiness hook so a new developer needs only `make up`. The equivalent production-capable definitions live in `infrastructure/terraform`.

Plan against LocalStack:

```sh
terraform -chdir=infrastructure/terraform init
terraform -chdir=infrastructure/terraform plan \
  -var='localstack=true' \
  -var='localstack_endpoint=http://localhost:4566'
```

For AWS, provide normal AWS credentials and omit the LocalStack variables. Queue URLs produced by Terraform should be supplied to the application through `CLAIM_COMMANDS_QUEUE_URL` and `CLAIM_EVENTS_QUEUE_URL`.

## Current scope

This first slice deliberately keeps claimant and claim functionality in one codebase while running the HTTP API, outbox publisher, worker, and frontend as independent processes. Authentication, policy-service integration, document storage, a working manual-review queue, payments, notifications, and PII encryption are not implemented yet.

## Deploying the portal to Vercel

Create a Vercel project with `frontend` as its root directory. Configure the server-only environment variable:

```text
OC_API_URL=https://your-deployed-go-api.example.com
```

The browser calls the portal's `/api/oc/*` route. Next.js forwards those requests to `OC_API_URL`, so the upstream address is not embedded in the client bundle. The deployed Go API must be reachable from Vercel; a service listening only inside local Docker cannot be used by a hosted frontend.

Do not deploy the current portal with real claimant data until authentication, authorization, production secrets, TLS, and appropriate PII controls have been added.
