# Runbook

Use this for local startup, migrations, env, proto generation, and CDC checks.

## Environment
- Example env: `deploy/env/chatserver.env.example`.
- Local env target: `deploy/env/chatserver.env`.
- Make target: `make env`.
- MySQL DSN should include:
  `timeout=5s&readTimeout=10s&writeTimeout=10s`.
- Do not record real secrets in `.codex`.

## Common Commands
- Start all local compose services: `make docker-up`.
- Stop compose services: `make docker-down`.
- Generate proto code: `make proto`.
- Install proto tools: `make tools`.
- Download modules: `make mod`.
- Tidy modules: `make tidy`.
- Build/test everything when practical: `go test ./...`.

## Single-Service Entrypoints
- Gateway: `go run ./apps/gateway/cmd`.
- Auth: `go run ./apps/auth/cmd`.
- User: `go run ./apps/user/cmd`.
- Relation: `go run ./apps/relation/cmd`.
- Group: `go run ./apps/group/cmd`.
- Msg: `go run ./apps/msg/cmd`.
- Connect: `go run ./apps/connect/cmd`.
- Message-push: `go run ./apps/message-push/cmd`.

## Default Ports And Addresses
- Gateway HTTP: `GATEWAY_ADDR`, default `:8080`.
- Auth gRPC/metrics: `AUTH_GRPC_ADDR=:9090`, `AUTH_METRICS_ADDR=:9190`.
- Connect HTTP/gRPC: `CONNECT_ADDR=:8081`, `CONNECT_GRPC_ADDR=:9091`.
- Msg gRPC/metrics: `MSG_GRPC_ADDR=:9092`, `MSG_METRICS_ADDR=:9192`.
- Relation gRPC/metrics: `RELATION_GRPC_ADDR=:9093`, `RELATION_METRICS_ADDR=:9193`.
- User gRPC/metrics: `USER_GRPC_ADDR=:9094`, `USER_METRICS_ADDR=:9194`.
- Group gRPC/metrics: `GROUP_GRPC_ADDR=:9095`, `GROUP_METRICS_ADDR=:9195`.
- Message-push metrics HTTP: `MESSAGE_PUSH_HTTP_ADDR`, example `:8084`.
- Connect route identity: `CONNECT_SELF_GRPC_ADDR` is required at runtime.

## Migrations
- Baseline schema: `config/mysql/001_schema.sql`.
- Incremental migrations: `scripts/migration`.
- Dead-letter table migration: `scripts/migration/004_dead_events.sql`.
- If adding consumer dead letters, update both model/schema and source naming docs.

## CDC / Kafka Connect
- Outbox depends on Debezium/Kafka Connect, not direct producer success.
- Connector script: `scripts/cdc/register_outbox_connector.sh`.
- Connector routes by outbox `event_type`.
- Env knobs include `KAFKA_CONNECT_URL`, `DEBEZIUM_CONNECTOR_NAME`,
  `DEBEZIUM_MYSQL_*`, `DEBEZIUM_TOPIC_PREFIX`, and `DEBEZIUM_OUTBOX_TABLE`.

## Troubleshooting
- Service starts but consumers do nothing: check Kafka Connect connector status and topic names.
- Consumer partition stuck: inspect logs, then query `dead_events` for pending records.
- WebSocket push missing: check Redis route hash, route freshness window, connect gRPC address,
  and `CONNECT_SELF_GRPC_ADDR`.
- Login/device state oddities: distinguish auth device-session snapshot from connect live route state.
