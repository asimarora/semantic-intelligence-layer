# SIL Architecture

SIL is a **service-oriented monorepo** built around a small number of coarse-grained services and a shared platform layer.

## Core runtime services

| Service | Responsibility |
| --- | --- |
| `ingestd` | ingest source events, validate envelopes, persist raw events, publish to JetStream |
| `normalizerd` | consume raw events, normalize into unified events, write metadata/read models |
| `indexerd` | asynchronous semantic text generation, vector indexing, and ML enrichment |
| `apid` | query, retrieval, and investigation APIs |
| `backfill` | historical replay through the same pipeline |
| `agentd` | future agent harness on top of SIL APIs and retrieval |

## Data and transport layers

| Layer | Role |
| --- | --- |
| Raw event store | append-only replay and audit source of truth |
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

## Agent harness

The agent harness sits **on top of** SIL, not inside the core data plane.

- SIL core remains useful without agents
- Agent workflows consume query, retrieval, and investigation capabilities
- Agent memory and approval logic remain separate from raw ingest and storage internals
