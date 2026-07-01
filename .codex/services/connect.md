# Connect Service

## Role
- Owns WebSocket access, live connection registry, Redis routing table,
  ACK handling, and internal push gRPC.

## Read First
- `apps/connect/cmd/providers.go`
- `apps/connect/internal/handler/ws_handler.go`
- `apps/connect/internal/manager`
- `apps/connect/internal/svc/auth.go`
- `apps/connect/internal/svc/lifecycle.go`
- `apps/connect/internal/svc/routing.go`
- `apps/connect/internal/svc/ack.go`
- `apps/connect/internal/grpc/server.go`
- `proto/connect`

## WebSocket Contract
- Endpoint: `GET /ws?token=...&device_id=...`.
- Live frames are binary protobuf `connect.MessageEnvelope`.
- Supported upstream types include heartbeat and `MSG_ACK` / `msg_ack`.
- Origin policy comes from `CONNECT_ALLOWED_ORIGINS`.
- Auth is fail-close; if Redis exists, access-token hash must match.
- JWT `device_id` must match query `device_id`.

## Routing And Lifecycle
- Route field is `device_id`; value is `connectGrpcAddr|lastActiveMs`.
- `CONNECT_SELF_GRPC_ADDR` is required for route identity and cleanup.
- On connect/heartbeat/disconnect, lifecycle touches active state and route storage.
- Device status RPC updates are queue-based and lossy under pressure by design.

## ACK Behavior
- ACK store is Redis-only.
- Key dimension: `user_uuid + device_id + conv_id`.
- Value is monotonic max acked seq with 30 day TTL.
- Only `MSG_PUSH` currently has enough metadata to attach conv_id for ACK watermarking.

## Traps
- Non-`MSG_PUSH` events with `ack_required` cannot become recordable unless metadata
  includes conv_id.
- If Redis is absent, routing and ACK reliability degrade sharply.
