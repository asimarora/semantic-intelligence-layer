# SIL Architecture

SIL is a **service-oriented monorepo** built around a small number of coarse-grained services and a shared platform layer.

## Core runtime services

| Service | Responsibility |
| --- | --- |
| `ingestd` | ingest source events, validate envelopes, persist raw events, publish to JetStream |
| `normalizerd` | consume raw events, normalize into unified events, write metadata/read models |
| `indexerd` | asynchronous semantic text generation, vector indexing, and ML enrichment |
| `apid` | query, retrieval, investigation APIs, and omnichannel webhook ingress |
| `backfill` | historical replay through the same pipeline |
| `agentd` | future agent harness on top of SIL APIs and retrieval |
| `modeld` | future inference gateway for vLLM, Triton, and other model backends |

## External access surfaces

SIL should keep **one intelligence core** while supporting multiple access
surfaces.

- `cmd/apid` remains the main entrypoint for external-facing APIs
- inbound omnichannel connectors live under `internal/channels/`
- API, assistant chat, webhook, and A2A adapters live under `internal/transport/`
- those adapters are presentation-layer code, not the home of business logic
- shared use cases should stay in `internal/services/` and `internal/agents/`

## Omnichannel support layer

For the first fully workable SIL foundation:

- **telecom evidence sources** remain under `internal/adapters/` (`radius` first)
- **customer entry channels** live under `internal/channels/`
- `chat`, `email`, `voice`, and `messaging` normalize into a shared raw interaction contract
- conversations, cases, identities, and timelines are modeled under `internal/contracts/unified/`
- `cmd/apid` is the initial runtime entrypoint for health, readiness, and webhook ingestion

## Model-serving plane

SIL should reserve a dedicated model-serving shell even before backend choices
are frozen.

- `cmd/modeld` is the future inference-facing service entrypoint
- `configs/models/` holds runtime and routing profiles
- `deployments/compose/models.yaml` and `deployments/k8s/inference/` reserve deployment paths
- `specs/models/` and `testdata/inference/` reserve request/profile contracts and fixtures
- **vLLM** and **Triton** are optional backends, not hard dependencies of the core data plane

## Agent control plane

The agent layer should stay framework-neutral by reserving SIL-owned runtime and
monitoring scaffolds:

- `internal/agents/runtime/` for durable run lifecycle and state transitions
- `internal/agents/executor/` for bounded execution loops, retry limits, and timeout budgets
- `internal/agents/approvals/` for human-in-the-loop approval flow
- `internal/platform/telemetry/` for low-level operational events
- `internal/platform/observability/` for degraded-quality and alerting semantics
- `internal/storage/metadata/{agents,toolcalls,retrieval,quality,approvals}/` for durable monitoring state
- `migrations/metadata/004_*` through `008_*` for the first agent-run and monitoring tables
- `specs/events/agent-run-event.json`, `tool-call-event.json`, `retrieval-event.json`, `quality-signal-event.json`, and `approval-event.json` for control-plane event contracts

## Data and transport layers

| Layer | Role |
| --- | --- |
| Raw event store | append-only replay and audit source of truth for evidence and inbound interaction records |
| Metadata store | structured filters, entity lookups, and correlation state |
| Vector store | semantic retrieval and nearest-neighbor search |
| NATS JetStream | durable asynchronous transport between services |

JetStream is the **service backbone**, not the system of record. Raw events must remain replayable even if downstream consumers are unavailable.

## Message topology

The current subject model is:

- `sil.raw.events`
- `sil.unified.events`
- `sil.index.jobs`
- `sil.deadletter`

All service-to-service messaging should flow through `internal/platform/messaging` so business logic stays broker-agnostic.

## Hot path vs async path

SIL should preserve a clear split:

1. **Hot path**
   - ingest
   - validation
   - raw event persistence
   - publication to JetStream

2. **Async path**
   - normalization
   - metadata projection
   - semantic text generation
   - vector indexing
   - ML scoring

This keeps ingestion reliable even when indexing, vector storage, or ML work is delayed.

## Future agent harness

The future agent harness sits **on top of** SIL, not inside the core data plane.

- SIL core remains useful without agents
- agent workflows consume query, retrieval, and investigation capabilities
- agent memory and approval logic remain separate from raw ingest and storage internals
