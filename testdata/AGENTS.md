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
- retrieval API: `POST /v1/retrieve`
- invoked investigation runs:
  - `POST /v1/agent-runs`
  - `GET /v1/agent-runs/{run_id}`
- watcher daemon via `cmd/agentd`

### 2. AAA auth / access logs

Implemented end-to-end through structured query:

- source config: `configs/sources/access.yaml`
- raw ingest adapter: `internal/adapters/access`
- ingest service: `internal/services/accessingest`
- unified access contract: `internal/contracts/unified/access`
- normalization service: `internal/services/accessnormalize`
- access projection store: `internal/storage/metadata/access`
- access query API: `GET /v1/access-events`

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
- The first agent flow is **deterministic and read-only**.
- `agentd` currently runs as a **polling watcher daemon** for repeated short-session disconnect detection.
- Local development uses **file-backed metadata** so separate processes can share projected state.

## Most important current gap

**AAA auth/access events are normalized and queryable, but they are not yet threaded into indexed retrieval or the investigation harness.**

That means:

- `indexerd` still only builds retrieval documents from normalized session events
- `POST /v1/retrieve` still effectively searches session-derived evidence
- the current investigation harness does not use access auth failures/challenges as first-class evidence

## Best next task

The best next implementation step is:

1. extend retrieval/indexing to include normalized access events
2. expose those access-derived documents through existing retrieval flows
3. enrich the investigation harness so auth failures, challenges, and policy decisions become agent evidence

Good starting files:

- `internal/services/index/service.go`
- `internal/contracts/unified/retrieval/types.go`
- `internal/services/query/retrieval.go`
- `internal/agents/harness/investigation.go`
- `internal/storage/metadata/access/store.go`
- `internal/services/accessnormalize/service.go`

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
go run ./cmd/normalizerd
go run ./cmd/indexerd
go run ./cmd/apid
go run ./cmd/agentd --once
```

### AAA auth / access flow

```bash
SIL_INGEST_SOURCE_TYPE=access SIL_INGEST_INPUT_PATH=./testdata/access go run ./cmd/ingestd
SIL_INGEST_SOURCE_TYPE=access SIL_INGEST_INPUT_PATH=./testdata/access go run ./cmd/normalizerd
curl -s "http://localhost:8080/v1/access-events?tenant_id=default&subscriber_id=john"
```

## Important files to read first

- `README.md`
- `cmd/ingestd/main.go`
- `cmd/normalizerd/main.go`
- `cmd/indexerd/main.go`
- `cmd/apid/main.go`
- `cmd/agentd/main.go`
- `internal/transport/api/server.go`
- `internal/agents/harness/investigation.go`
- `internal/agents/runtime/daemon.go`

## Resume note

If resuming work after a gap, assume the repo is currently at:

- RADIUS accounting: indexed + retrievable + agent-consumable
- AAA auth/access: normalized + projected + queryable
- next milestone: **make access events retrievable and agent-consumable**
