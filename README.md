# Semantic Intelligence Layer (SIL)

SIL is a **source-agnostic intelligence platform** that sits above event-producing systems such as **RAS**. It ingests structured events through source-specific adapters, normalizes them into a unified event model, generates semantic representations and embeddings, and exposes retrieval and analysis APIs for downstream applications, analysts, and future agentic systems.

While the architecture is source-agnostic, the near-term product direction is **telecom-first**. RAS is the initial source because it provides a strong subscriber and session backbone that other telecom signals can attach to later.

## Repository status

This repository now contains the **target architecture plus the first working telecom pipeline slices**.

- **Implementation status:** early executable foundation with working raw RAS ingest and first-pass session normalization
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

## Telecom-first direction

SIL is designed to be reusable across many domains, but its strongest early fit is telecom and service-provider operations.

Early telecom value comes from turning isolated records into:

- subscriber intelligence
- session intelligence
- network element correlation
- blast-radius analysis
- service-impact investigations
- reusable operational memory

Representative telecom questions SIL should eventually answer:

- why is this subscriber repeatedly dropping sessions?
- which NAS, site, or software version is impacting the most subscribers?
- which short-session patterns correlate with DNS, DHCP, or auth failures?
- which tickets match an active network-side incident?
- which behaviors suggest fraud, abuse, or policy misconfiguration?

## Why RAS is the right first source

RAS is a strong first source because it already gives SIL the session backbone many telecom investigations need.

1. **Subscriber and session identity**
   RAS provides usernames and session identifiers that can anchor correlation.

2. **Network edge context**
   NAS information gives a direct join point into access-network devices and sites.

3. **Session lifecycle**
   Start, stop, and interim events reveal session behavior over time.

4. **Usage characteristics**
   Session duration and octet counters provide immediate behavioral signals.

5. **Operationally safe ingestion**
   Mirrored accounting traffic lets SIL start without changing the live auth plane.

On its own, RAS already supports similarity search, anomaly scoring, and repeated-session analysis. Combined with carefully selected adjacent sources, it becomes the core join spine for deeper telecom intelligence.

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

## Source expansion should be value-driven

A new source should be added only if it improves one or more of these:

- **entity linkage** between subscriber, session, IP, NAT mapping, network element, and case
- **root-cause precision** for repeated drops, failed sessions, or degraded service
- **customer-impact visibility** across tickets, regions, and affected subscriber groups
- **operational actionability** so results can point to a device, policy, workflow, or team
- **historical replay value** so the source is worth storing and reprocessing over time

High-volume feeds that cannot be joined reliably to subscriber, session, device, or service entities should not be early priorities.

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

## High-value telecom sources after RAS

Not every telecom feed adds the same value. The best next sources are the ones that make RAS more explainable, more actionable, and easier to correlate.

| Source | Priority after RAS | Value added | Why it is worth integrating |
| --- | --- | --- | --- |
| **AAA auth / access logs** | Very high | Failed logins, reject reasons, policy context before accounting starts | Completes the RADIUS story and explains sessions that never become stable accounting records |
| **DHCP / IPAM** | Very high | Subscriber-to-IP lease history | Essential for correlating sessions with IP lifecycle, DNS activity, abuse cases, and support workflows |
| **BNG / BRAS / NAS syslogs and alarms** | Very high | Device-side failure evidence | Connects repeated session churn to real network-side faults instead of treating them as isolated subscriber issues |
| **DNS resolver logs** | High | Service symptom visibility | Explains "connected but not working" cases and helps tie access sessions to application or reachability problems |
| **Inventory / topology** | High | Site, region, vendor, software, and dependency context | Enables blast-radius analysis and issue grouping by hardware, software version, or location |
| **CGNAT logs** | High | Subscriber-to-public-IP and port mapping | High value for abuse handling, forensics, and downstream service correlation |
| **NetFlow / IPFIX / sFlow** | Medium-high | Traffic behavior and QoE clues | Helps distinguish idle churn, abnormal usage, congestion, and suspicious behavior patterns |
| **CRM / trouble tickets** | Medium-high | Customer-impact and outcome loop | Links technical patterns to human impact and builds reusable operational memory |
| **PCRF / PCF / OCS / charging** | Medium | Policy, quota, and charging context | Useful when throttling, quota exhaustion, or charging side effects are major causes of complaints |
| **IMS / SIP / CDR / xDR** | Medium | Voice and service-specific intelligence | Important when SIL expands into mobile-core or voice-service investigations |

## Suggested telecom expansion order

For a general telecom or ISP rollout, the strongest sequence after RAS is:

1. **AAA auth / access logs**
2. **DHCP / IPAM**
3. **BNG / BRAS / NAS syslogs and alarms**
4. **DNS resolver logs**
5. **Inventory / topology**
6. **CGNAT logs**
7. **NetFlow / IPFIX / sFlow**
8. **CRM / trouble tickets**
9. **PCRF / PCF / OCS / charging**
10. **IMS / SIP / CDR / xDR**

This order is based on correlation value, root-cause improvement, and operational usefulness.

- If SIL stays focused on **fixed broadband**, DHCP, DNS, BNG alarms, and topology should be emphasized early.
- If SIL expands faster into **mobile or voice**, policy, charging, IMS, and CDR/xDR should move up the list.

## Core telecom entity model

As SIL grows beyond RAS, the most important cross-source entities are:

- subscriber or account
- session
- device or CPE or handset
- IP lease and NAT mapping
- network element, site, and region
- service event
- policy or charging state
- incident or trouble ticket

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

The `transport/` packages are the private home for future external access
surfaces such as structured APIs, chat, MCP, and A2A adapters. They sit behind
`cmd/apid` or other entrypoints and should call shared services rather than
re-implementing intelligence logic.

The `channels/` packages are the omnichannel ingress layer for customer-facing
entry points such as chat, email, voice, and messaging. They normalize channel
payloads into SIL contracts without replacing the telecom evidence-source model
under `internal/adapters/`.

The outer repository shape also reserves a **model-serving plane**. `cmd/modeld`
is the future inference gateway, `configs/models/` holds backend and routing
profiles, and deployment/spec/testdata folders reserve space for **vLLM**,
**Triton**, and later inference backends without forcing SIL to depend on any
single runtime today.

The repo now also reserves an **agent control-plane and monitoring shell**.
`internal/agents/runtime`, `executor`, and `approvals` represent the bounded
execution loop and approval flow; `internal/platform/telemetry` and
`observability` reserve shared monitoring hooks; and the new metadata, event,
migration, and fixture files reserve durable storage for agent runs, tool calls,
retrieval misses, quality signals, and approval decisions.

The repo also now reserves **Temporal orchestration** and **MCP integration**
shells. `internal/agents/runtime/temporal/` is the future durable workflow
adapter, `internal/integrations/mcp/` is the outbound MCP client and registry
area for backend systems, `internal/agents/tools/mcp/` is the bridge into the
SIL tool registry, and `internal/transport/mcp/` reserves a future MCP-server
surface if SIL later exposes its own tools outward.

The `agents/` and `agentd/` scaffolds intentionally reserve space for a future agent harness on top of SIL. The initial contracts now cover run/session models, tool and memory boundaries, and a starter policy evaluator for read-only, approval-gated, and tenant-isolated actions, but they do **not** change the core rule that SIL's ingest, normalization, storage, and messaging layers remain useful without any agent runtime.

The root `compose.yaml` is intended as a lightweight local infrastructure stack for development. It brings up shared dependencies such as NATS JetStream, Postgres, and Redis without forcing the whole runtime into Docker before the core services are ready.

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

- GraphRAG over entities such as subscriber, session, NAS, IP, site, ticket, and network event
- investigation agents that gather evidence across retrieval modes
- root cause hypothesis generation
- recommendation agents for runbooks and playbooks
- human approval gates for high-impact actions
- MCP or tool-based interfaces on top of a stable query API

## ML capabilities to prioritize

When SIL moves beyond retrieval, the highest-value ML additions are:

1. **Streaming anomaly detection** for live telemetry and changing baselines
2. **Behavioral clustering** for subscribers, NAS devices, sessions, and traffic patterns
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

It starts with **RAS as the first source**, but it is designed to support many sources over time. In the near term, SIL is best understood as a **telecom-first subscriber, session, and network intelligence layer** with RAS as the initial correlation backbone. The long-term direction is an AI-native platform with memory, retrieval, ML, and carefully controlled agentic workflows.
