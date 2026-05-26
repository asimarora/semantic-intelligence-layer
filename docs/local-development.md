# Local Development

## Prerequisites

- Go 1.22+
- Docker with Compose support (`docker compose` or `docker-compose`)

## Quick start

Compile everything:

```bash
make build
```

Run tests:

```bash
make test
```

Start the minimal local infrastructure stack:

```bash
make dev-up
```

Start the self-contained demo stack:

```bash
make demo-up
```

Seed the local cross-source demo data:

```bash
make demo-cross-source-prepare
```

Seed the local cross-source demo data and start the API:

```bash
make demo-cross-source
```

Stop the self-contained demo stack:

```bash
make demo-down
```

Stop the minimal local infrastructure stack:

```bash
make dev-down
```

Start the richer Docker stack infrastructure:

```bash
make dev-full-up
```

Build and start the richer Docker app profile:

```bash
make dev-app-up
```

Stop the richer Docker stack:

```bash
make dev-full-down
```

## Compose files

### Root `compose.yaml`

The root compose file is the **minimal local infrastructure stack**. It currently starts:

- NATS with JetStream
- Postgres
- Redis

### `deployments/compose/demo.yaml`

This is the **smallest Docker demo**. It builds a single image that:

1. ingests the bundled RADIUS sample
2. normalizes it
3. indexes it
4. starts `apid` on port `8080`

Use it when you want a copy-paste demo without manually running each command.

`POST /v1/retrieve` is a **read-only search endpoint**. It uses `POST` because retrieval accepts a structured JSON body with the free-text query plus optional filters such as subscriber, session, outcome, IP addresses, and time range; it does not create a resource.

Example:

```bash
make demo-up
curl "http://127.0.0.1:8080/v1/sessions?tenant_id=default&subscriber_id=john"
curl -X POST "http://127.0.0.1:8080/v1/retrieve" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","query":"john disconnected","limit":5}'
make demo-down
```

## Cross-source local demo

The next demo slice is a **single-subscriber cross-source timeline** that combines AAA access decisions with RADIUS session events.

The bundled fixtures currently model:

1. an access reject for `john` because `invalid password`
2. a later access accept for the same subscriber and session
3. a RADIUS session start for `sess-001`
4. a RADIUS session stop after a short session

Prepare the demo state:

```bash
make demo-cross-source-prepare
```

Then start the API in another terminal:

```bash
SIL_APP_ENV=development go run ./cmd/apid
```

Or do both in one command:

```bash
make demo-cross-source
```

Query the access and session sides separately:

```bash
curl "http://127.0.0.1:8080/v1/access-events?tenant_id=default&subscriber_id=john"
curl "http://127.0.0.1:8080/v1/sessions?tenant_id=default&subscriber_id=john"
```

Query the retrieval index across both evidence types:

```bash
curl -X POST "http://127.0.0.1:8080/v1/retrieve" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","query":"invalid password","subscriber_id":"john","limit":5}'

curl -X POST "http://127.0.0.1:8080/v1/retrieve" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","query":"started a network session","subscriber_id":"john","limit":5}'
```

Run the correlated investigation demo:

```bash
curl -X POST "http://127.0.0.1:8080/v1/agent-runs" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","goal":"Did john fail auth before connecting?","query":"john invalid password then connected","filters":{"subscriber_id":"john"},"max_steps":4}'
```

Or use the browser chat interface:

```text
http://127.0.0.1:8080/chat
```

The WebSocket chat demo sends the question to the same deterministic investigation core used by `POST /v1/agent-runs`. The default form values are already set to the bundled `john` cross-source demo.

The investigation harness now performs:

1. indexed evidence retrieval
2. normalized access enrichment
3. normalized session enrichment
4. a deterministic cross-source summary

For the bundled fixtures, the expected summary is the timeline:

1. rejected at `13:55` because `invalid password`
2. accepted at `13:58`
3. session `sess-001` started at `13:58:20`
4. session `sess-001` stopped at `14:00`

This is the current **cross-source demo surface**. The next gap is reusing the same correlation logic in watcher rules and adding an A2A transport over the same reasoning core.

### `deployments/compose/local.yaml`

This is the richer local stack for service-oriented development. It includes the same infrastructure and adds optional service containers behind the `app` profile.

The development configuration now uses **file-backed metadata** under `./var/development/metadata`, and the Docker app services share `/app/var` through a named volume so ingest, normalization, indexing, and API reads can see the same local state.

Example:

```bash
docker compose -f deployments/compose/local.yaml up -d
docker compose -f deployments/compose/local.yaml --profile app up -d --build
```

## Common Make targets

- `make build` compiles all packages
- `make build-bins` writes binaries to `./bin`
- `make run-apid` runs a specific command from `./cmd`
- `make demo-up` / `make demo-down` / `make demo-logs` / `make demo-ps` manage the self-contained demo stack
- `make demo-cross-source-prepare` / `make demo-cross-source` seed and run the local cross-source demo
- `make dev-up` / `make dev-down` manage the root infra stack
- `make dev-full-up` / `make dev-app-up` / `make dev-full-down` manage the richer Docker stack
- `make models-up` / `make models-down` manage the model-serving compose profile

## Config environments

The current environment set is:

- `development`
- `testing`
- `staging`
- `production`

For local runs, the default is `development`.

To override:

```bash
SIL_APP_ENV=staging go run ./cmd/backfill
```

## Notes

- `testing` is for deterministic automated test behavior, not pre-production parity
- `staging` is the production-like validation environment
- JetStream should be treated as transport, not the long-term source of truth
