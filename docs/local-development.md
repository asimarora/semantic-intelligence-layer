# Local Development

## Prerequisites

- Go 1.22+
- Docker with `docker compose`

## Quick start

Start local infrastructure:

```bash
make dev-up
```

Run tests:

```bash
make test
```

Stop local infrastructure:

```bash
make dev-down
```

## Compose files

### Root `compose.yaml`

The root compose file is the **minimal local infrastructure stack**. It currently starts:

- NATS with JetStream
- Postgres
- Redis

### `deployments/compose/local.yaml`

This is the richer local stack for service-oriented development. It includes the same infrastructure and adds optional service containers behind the `app` profile.

Example:

```bash
docker compose -f deployments/compose/local.yaml up -d
docker compose -f deployments/compose/local.yaml --profile app up -d
```

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
