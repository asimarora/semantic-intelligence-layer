# Semantic Intelligence Layer (SIL)

SIL is a **source-agnostic intelligence platform** that sits above event-producing systems such as **RAS**. It ingests structured events through source-specific adapters, normalizes them into a unified event model, generates semantic representations and embeddings, and exposes retrieval and analysis APIs for downstream applications, analysts, and future agentic systems.

## Repository status

This repository currently captures the **target architecture and planned v1 design**.

- **Implementation status:** design/specification stage
- **Initial source system:** RAS
- **Primary v1 goal:** semantic ingestion and retrieval
- **Not required in v1:** MCP server, autonomous remediation, full agent orchestration

## System position

SIL is intentionally separate from upstream data planes.

| System | Responsibility |
| --- | --- |
| **RAS** | Ingest mirrored RADIUS accounting traffic, store source records, expose real-time and historical feeds |
| **SIL** | Normalize source events, build semantic representations, generate embeddings, support retrieval, analysis, and future AI workflows |

This separation keeps protocol- and source-specific logic in the source system and keeps intelligence, retrieval, and future AI-native capabilities inside SIL.

## Why SIL exists

Traditional telemetry systems are strong at deterministic ingestion and storage but weak at:

- semantic similarity search
- behavioral pattern discovery
- cross-source correlation
- reusable case memory
- AI-assisted investigation

SIL adds that missing intelligence layer without forcing upstream systems to change.

## Design principles

1. **Source-agnostic**
   Adapters isolate source-specific logic so the core platform remains reusable across RAS, logs, chat, VoIP, and other telemetry feeds.

2. **Non-intrusive**
   SIL operates asynchronously and does not sit in the critical path of upstream systems.

3. **Reprocessable**
   Raw events, normalized events, metadata, and embeddings should be separable so data can be replayed and re-embedded when schemas or models change.

4. **Model-agnostic**
   Embedding and inference layers should support multiple providers and model families.

5. **Explainable by design**
   Retrieval and future AI features should return evidence, reasons, and traceable context instead of opaque scores only.

6. **Safe evolution**
   SIL should grow from retrieval and intelligence into agentic workflows gradually, with explicit guardrails and human approval where needed.

## Planned v1 scope

Version 1 should focus on the minimum platform needed to make RAS data semantically useful.

1. **RAS adapter** for real-time ingestion from Redis and historical ingestion from logs/files
2. **Unified event model** for source-agnostic processing
3. **Semantic text generation** from structured events
4. **Embedding pipeline** with pluggable model providers
5. **Durable storage layers** for raw events, metadata, and vectors
6. **Hybrid retrieval API** for similarity search with filters
7. **Backfill and replay support** for historical processing

## Explicit non-goals for v1

- Replacing RAS as the source-specific ingestion system
- Building an MCP server first
- Autonomous remediation or infrastructure-changing actions
- Full free-form LLM response generation as the main interface
- Tight coupling to a single embedding model, vector store, or cloud vendor

## End-to-end architecture

```text
RAS / Other Sources
    |
    |-- Real-time feed (Redis, stream, queue)
    |-- Historical feed (logs, exports, files)
    v
Adapter Layer
    v
Validation / Dedupe / Checkpointing
    v
Unified Event Model
    v
Semantic Representation
    v
Embedding Pipeline
    v
+-------------------+-------------------+------------------+
| Raw Event Store   | Metadata Store    | Vector Store     |
| replay / audit    | filters / lineage | ANN retrieval    |
+-------------------+-------------------+------------------+
    v
Hybrid Retrieval + Reranking
    v
Query API
    v
Analysts / Apps / Future Agents
```

## Core platform components

### 1. Adapter layer

Adapters translate source-native payloads into SIL-ready inputs.

- Converts source-specific records into a common envelope
- Handles transport-specific concerns such as Redis consumption, log parsing, checkpointing, and replay
- Initial adapter: **RAS**

### 2. Validation and ingestion control

This layer ensures the platform is robust and replayable.

- schema validation
- deduplication
- ordering or checkpoint tracking where possible
- ingest metadata such as source, transport, version, and ingest timestamp

### 3. Unified event model

All downstream processing should operate on a common representation rather than source-native payloads.

Example:

```json
{
  "event_id": "ras:sess-001:2026-05-04T14:00:00Z",
  "source": "ras",
  "event_type": "network.session.accounting",
  "timestamp": "2026-05-04T14:00:00Z",
  "entities": {
    "user": "john",
    "session_id": "sess-001",
    "nas_ip": "192.168.1.1",
    "client_ip": "10.0.0.25"
  },
  "attributes": {
    "acct_status_type": "Accounting-Stop",
    "acct_session_time": 100,
    "acct_input_octets": 12000,
    "acct_output_octets": 54000
  },
  "tags": [
    "radius",
    "disconnect"
  ],
  "semantic_text": "user john disconnected from NAS 192.168.1.1 after a 100 second session with 12000 input octets and 54000 output octets"
}
```

### 4. Semantic representation

Structured events should be transformed into consistent, semantically meaningful text or multi-field representations suitable for embedding and retrieval.

Example:

Input:

```text
username=john, nas=192.168.1.1, session_time=100, status=Accounting-Stop
```

Output:

```text
user john disconnected from NAS 192.168.1.1 after a 100 second session
```

### 5. Embedding pipeline

The embedding layer converts semantic representations into vectors.

- pluggable provider interface
- batch and streaming support
- model version tracking
- re-embedding support when prompts, schemas, or models change

### 6. Storage layers

SIL should separate operational storage responsibilities instead of overloading one system.

| Layer | Purpose | Notes |
| --- | --- | --- |
| **Raw event store** | Replay, audit, backfill, source traceability | Durable copy of source payloads or normalized envelopes |
| **Metadata store** | Structured filters, timestamps, entity lookup, lineage | SQL or document-oriented store |
| **Vector store** | Embedding storage and nearest-neighbor retrieval | Milvus is the current preferred direction |
| **Redis (optional)** | Streaming buffer, cache, checkpointing, short-lived operational state | Useful for low-latency paths, but not the long-term semantic source of truth |

### 7. Retrieval engine

Retrieval in SIL should be **hybrid**, not vector-only.

Planned retrieval features:

- vector similarity search
- metadata filtering
- time-window filtering
- keyword or full-text augmentation
- reranking
- explanation-friendly evidence packaging

### 8. Query API

The query API exposes SIL capabilities to applications and future tools.

Representative queries:

- `sessions similar to this disconnect pattern`
- `users with repeated short sessions`
- `NAS devices showing abnormal behavior this week`
- `show prior cases similar to this session signature`

## Data contracts

SIL should keep multiple views of the same data for different jobs.

### Raw source event

```json
{
  "source": "ras",
  "transport": "redis",
  "ingested_at": "2026-05-04T14:00:01Z",
  "source_key": "radius:acct:john:sess-001:20260504T140000.000000",
  "payload": {
    "username": "john",
    "nas_ip_address": "192.168.1.1",
    "acct_status_type": "Accounting-Stop",
    "acct_session_id": "sess-001",
    "acct_session_time": 100,
    "acct_input_octets": 12000,
    "acct_output_octets": 54000,
    "timestamp": "2026-05-04T14:00:00Z"
  }
}
```

### Unified event

```json
{
  "event_id": "ras:sess-001:2026-05-04T14:00:00Z",
  "source": "ras",
  "event_type": "network.session.accounting",
  "timestamp": "2026-05-04T14:00:00Z",
  "entities": {
    "user": "john",
    "session_id": "sess-001",
    "nas_ip": "192.168.1.1"
  },
  "attributes": {
    "status": "Accounting-Stop",
    "session_time_seconds": 100
  },
  "semantic_text": "user john disconnected from NAS 192.168.1.1 after a 100 second session"
}
```

### Retrieval result

```json
{
  "query": "sessions similar to this disconnect pattern",
  "matches": [
    {
      "event_id": "ras:sess-884:2026-05-03T11:20:00Z",
      "score": 0.91,
      "matched_by": [
        "vector",
        "time-filter",
        "metadata-filter"
      ],
      "explanation": "Similar disconnect event, short session duration, and comparable traffic profile",
      "semantic_text": "user jane disconnected from NAS 192.168.1.1 after a 95 second session"
    }
  ]
}
```

## Planned repository structure

The likely first repo shape is a modular monolith, with adapters isolated at the edge and reusable core logic in the platform.

```text
semantic-intelligence-layer/
├── README.md
├── docs/
├── configs/
├── schemas/
├── apps/
│   ├── api/
│   ├── worker/
│   └── backfill/
├── src/
│   └── sil/
│       ├── adapters/
│       │   └── radius/
│       ├── ingestion/
│       ├── normalization/
│       ├── semantics/
│       ├── embeddings/
│       ├── storage/
│       ├── retrieval/
│       ├── ml/
│       ├── agents/
│       └── config/
├── tests/
└── deployments/
```

## AI-native roadmap

SIL is meant to evolve from a semantic retrieval platform into an AI-native investigation platform.

### Now

- source adapters
- unified event model
- semantic text generation
- embeddings
- hybrid retrieval
- evidence-backed query APIs

### Next

- anomaly scoring
- behavioral clustering
- case memory from prior investigations
- analyst feedback loops
- retrieval reranking
- drift monitoring for schemas and embeddings

### Later

- GraphRAG over entities such as user, session, NAS, IP, and event
- investigation agents that gather evidence across retrieval modes
- root cause hypothesis generation
- recommendation agents for runbooks and playbooks
- human approval gates for high-impact actions
- MCP or tool-based interfaces on top of a stable query API

## ML capabilities to prioritize

When SIL moves beyond retrieval, the highest-value ML additions are:

1. **Streaming anomaly detection** for live telemetry and changing baselines
2. **Behavioral clustering** for users, NAS devices, sessions, and traffic patterns
3. **Sequence modeling** for repeated lifecycle or disconnect patterns
4. **Similarity learning** tailored to incident or session behavior, not just generic embeddings
5. **Feedback learning** from analyst labels such as useful, false positive, same pattern, or different root cause
6. **Time-aware scoring** so recent behavior can be weighted differently from long-term history

## Agentic capabilities to add carefully

If SIL grows into an agentic platform, the safest progression is:

1. **Triage agent** to cluster and prioritize incoming anomalies
2. **Investigation agent** to gather related sessions, entities, and prior cases
3. **RCA agent** to generate grounded root-cause hypotheses
4. **Recommendation agent** to suggest actions or runbooks
5. **Approval-gated action layer** only after clear auditability and control are in place

## Evaluation and guardrails

To be useful in production, SIL should be measured and governed explicitly.

### Evaluation

- retrieval relevance such as recall at K and ranking quality
- latency for ingest, indexing, and query paths
- groundedness and evidence coverage for generated or summarized outputs
- anomaly precision and operator usefulness
- drift detection across schemas, embeddings, and data quality

### Guardrails

- audit trail for ingest, retrieval, and future agent actions
- PII-aware handling and retention policies
- explicit model and prompt versioning
- human approval for high-risk or state-changing workflows
- clear separation between recommendation and execution

## Summary

SIL is the platform layer that turns source telemetry into **semantic, retrievable, explainable intelligence**.

It starts with **RAS as the first source**, but it is designed to support many sources over time. The near-term focus is a strong semantic ingestion and retrieval foundation. The long-term direction is an AI-native platform with memory, retrieval, ML, and carefully controlled agentic workflows.
