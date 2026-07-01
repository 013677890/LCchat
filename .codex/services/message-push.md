# Message-Push Service

## Role
- Consumes Kafka `msg.push`.
- Reads routes from Redis, expands group members through group-service, and calls
  connect-service gRPC for live delivery.

## Read First
- `apps/message-push/cmd/providers.go`
- `apps/message-push/internal/consumer/consumer.go`
- `apps/message-push/internal/consumer/realtime_handler.go`
- `apps/message-push/internal/route/repository.go`
- `apps/message-push/internal/connectcli`
- `proto/msg/msg_push_event.proto`
- `proto/realtime/realtime_event.proto`

## Delivery Behavior
- P2P `MSG_PUSH` / `MSG_RECALL`: receiver routes plus sender's other devices.
- Group `MSG_PUSH` / `MSG_RECALL`: expand group members except sender, then sync sender's other devices.
- `MSG_MARK_READ`: same user's other devices.
- `MSG_READ_RECEIPT`: receiver side routes only.
- Ack is required only for `MSG_PUSH` with `seq > 0`.
- Routes are de-duplicated by `user_uuid + device_id` before connect calls.
- Event is considered handled if at least one target device push succeeds.

## Retry Behavior
- This service uses its own loop, not `pkg/kafka.NewManualCommitConsumer`.
- Local retry is finite: 3 attempts with short backoff.
- Each attempt now has a broad timeout budget to cap pathological fan-out.
- After retries fail, it still commits the offset to avoid blocking the partition.
- It does not currently write `dead_events`.

## Traps
- `MSG_PUSH` can rely on seq gap pull as a client-side fallback.
- Recall/read/realtime events with `seq==0` have weaker replay/dedupe semantics.
- Redis route read filters stale routes by active timestamp even if Redis cleanup lags.
