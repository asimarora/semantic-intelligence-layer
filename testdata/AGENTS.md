# SIL Copilot Init

## What this repository is

Semantic Intelligence Layer (**SIL**) is a **Go-first, telecom-first, agentic backend intelligence platform**.

The current goal is to build deterministic, evidence-backed telecom intelligence in this order:

1. source-specific ingest
2. unified normalization
3. durable metadata projection
4. structured retrieval
5. indexed retrieval
6. bounded agent investigations
7. watcher/daemon-triggered investigations

Do not jump straight to free-form LLM flows or autonomous actions when a structured, evidence-backed slice is still missing.

## Current implemented slices

### 1. RADIUS / RAS accounting

Implemented end-to-end:

- raw ingest via `cmd/ingestd`
- raw evidence store under `internal/storage/raw`
- normalized session events under `internal/contracts/unified/sessions`
- session projection store under `internal/storage/metadata/sessions`
- session query API: `GET /v1/sessions`
- indexed retrieval via `cmd/indexerd`
- retrieval API: `POST /v1/retrieve` (read-only search with structured JSON filters)
- invoked investigation runs:
  - `POST /v1/agent-runs`
  - `GET /v1/agent-runs/{run_id}`
- watcher daemon via `cmd/agentd`

### 2. AAA auth / access logs

Implemented end-to-end through retrieval and investigation evidence:

- source config: `configs/sources/access.yaml`
- raw ingest adapter: `internal/adapters/access`
- ingest service: `internal/services/accessingest`
- unified access contract: `internal/contracts/unified/access`
- normalization service: `internal/services/accessnormalize`
- access projection store: `internal/storage/metadata/access`
- access query API: `GET /v1/access-events`
- indexed retrieval via `cmd/indexerd`
- retrieval API exposure: `POST /v1/retrieve` (read-only search with structured JSON filters)
- investigation evidence via `POST /v1/agent-runs`

### 3. Logging

Implemented:

- bounded async per-service structured logging
- flush-on-shutdown runtime
- optional mirrored JSONL service logs
- local OTel collector wiring for development

Key code:

- `internal/platform/logging/logger.go`
- `internal/platform/logging/async_handler.go`
- `deployments/compose/otel-collector-config.yaml`

## Current platform behavior

- Tenant isolation is currently **strong logical isolation**, not hard physical isolation.
- Retrieval is currently **deterministic lexical retrieval**, not vector-backed yet.
- Retrieval uses `POST /v1/retrieve` for **read-only search** because the request body carries structured query filters.
- The first agent flow is **deterministic and read-only**.
- `agentd` currently runs as a **polling watcher daemon** for repeated short-session disconnect detection and now prioritizes subscribers with correlated access-plus-session evidence in the same window.
- Local development uses **file-backed metadata** so separate processes can share projected state.
- Development now enables both `radius` and `access` sources so a single local demo can seed cross-source evidence.
- `apid` now serves a **WebSocket chat demo** at `/chat` backed by the same deterministic investigation core as `POST /v1/agent-runs`.

## Most important current gap

**The harness, chat demo, and watcher daemon now share the same cross-source access-plus-session reasoning core, but the A2A layer and lighter bounded question routing still lag behind it.**

That means:

- `POST /v1/retrieve` returns both session and access evidence, but ranking is still deterministic lexical retrieval
- `POST /v1/agent-runs` now enriches directly from normalized access and normalized session stores, then produces a deterministic correlated summary for the current window
- `/chat` now provides a browser-based WebSocket test interface over the same bounded investigation flow
- `agentd` now enriches repeated short-session candidates with normalized access evidence and prioritizes subscribers whose access and session timelines correlate in the current window

## Best next task

The best next implementation step is:

1. expose the shared reasoning core through an A2A transport without splitting it by interface
2. improve the bounded question layer so chat/A2A can answer common operator asks with lighter-weight routing when a full investigation is unnecessary
3. move retrieval from deterministic lexical search toward embeddings plus hybrid ranking

Good starting files:

- `internal/agents/harness/investigation.go`
- `internal/agents/runtime/daemon.go`
- `internal/transport/a2a/doc.go`
- `internal/transport/chat/handler.go`

## Working rules for this repo

- Keep changes **Go-native, deterministic, and replayable**.
- Reuse existing config, logging, messaging, and storage patterns instead of inventing parallel abstractions.
- Preserve **tenant scoping** on every API, projection, retrieval path, and agent step.
- Prefer **source expansion that improves evidence quality** over speculative orchestration work.
- Keep agents **evidence-backed and bounded**.
- Do not introduce silent fallbacks; surface real errors.

## Useful commands

### Validation

```bash
go test ./...
go build ./...
```

### RADIUS flow

```bash
go run ./cmd/ingestd
go run ./cmd/normalized
go run ./cmd/indexerd
go run ./cmd/apid
go run ./cmd/agentd --once
```

### AAA auth / access flow

```bash
SIL_INGEST_SOURCE_TYPE=access SIL_INGEST_INPUT_PATH=./testdata/access go run ./cmd/ingestd
SIL_INGEST_SOURCE_TYPE=access SIL_INGEST_INPUT_PATH=./testdata/access go run ./cmd/normalized
SIL_INGEST_SOURCE_TYPE=access SIL_INGEST_INPUT_PATH=./testdata/access go run ./cmd/indexerd
curl -s "http://localhost:8080/v1/access-events?tenant_id=default&subscriber_id=john"
```

### Cross-source demo flow

```bash
make demo-cross-source-prepare
go run ./cmd/apid
curl -s "http://localhost:8080/v1/access-events?tenant_id=default&subscriber_id=john"
curl -s "http://localhost:8080/v1/sessions?tenant_id=default&subscriber_id=john"
curl -s -X POST "http://localhost:8080/v1/retrieve" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","query":"invalid password","subscriber_id":"john","limit":5}'
curl -s -X POST "http://localhost:8080/v1/agent-runs" \
  -H "Content-Type: application/json" \
  -d '{"tenant_id":"default","goal":"Did john fail auth before connecting?","query":"john invalid password then connected","filters":{"subscriber_id":"john"},"max_steps":4}'
open http://localhost:8080/chat
```

## Important files to read first

- `README.md`
- `cmd/ingestd/main.go`
- `cmd/normalized/main.go`
- `cmd/indexerd/main.go`
- `cmd/apid/main.go`
- `cmd/agentd/main.go`
- `internal/transport/api/server.go`
- `internal/agents/harness/investigation.go`
- `internal/agents/runtime/daemon.go`

## Resume note

If resuming work after a gap, assume the repo is currently at:

- RADIUS accounting: indexed + retrievable + agent-consumable
- AAA auth/access: normalized + projected + indexed + retrievable + agent-consumable
- next milestone: **correlate access auth evidence with session outcomes inside the harness**
