# Watchlist

Known traps and mismatch areas. Each item answers what breaks, when to check,
and where to inspect first.

## User Redis Fallback
- What breaks: user-service may panic or fail paths that assume Redis is non-nil.
- When to check: claiming MySQL-only fallback, changing providers, or running user-service
  without Redis.
- Inspect first: `apps/user/cmd/providers.go`, `apps/user/internal/repository/user_repository.go`.
- Confidence: high; verify current code before changing the note.

## Conversation Identity Drift
- What breaks: duplicate/upsert semantics for conversation rows can drift across DB dialects
  or schema changes.
- When to check: changing `conversation` indexes, `conv_id`, target UUID semantics, or upsert code.
- Inspect first: `config/mysql/001_schema.sql`,
  `apps/msg/internal/domain/conversation/repository_impl.go`.
- Confidence: high historically; re-verify after schema migrations.

## Seq Holes Are Possible
- What breaks: assuming max seq equals contiguous persisted messages.
- When to check: read repair, gap pull, max-seq APIs, ACK logic.
- Inspect first: `apps/msg/internal/domain/message/service.go`,
  `apps/msg/internal/domain/message/repository_impl.go`.
- Confidence: high; Redis INCR can happen before DB insert succeeds.

## Manual Consumer Exactly-Once Illusion
- What breaks: non-idempotent side effects can run twice if crash/rebalance happens after
  business work but before `MarkIdempotent`.
- When to check: adding new outbox consumers or side effects such as counters, email, money,
  notifications, or external calls.
- Inspect first: `pkg/outbox/store.go`, existing consumers under `apps/*/internal/consumer`.
- Confidence: high; current consumers mostly work because handlers are idempotent.

## Dead-Letter Does Not Mean Replay Is Implemented
- What breaks: operators may assume pending `dead_events` automatically replay.
- When to check: adding DLQ, writing runbooks, or designing admin tools.
- Inspect first: `pkg/outbox/deadletter.go`, `scripts/migration/004_dead_events.sql`.
- Confidence: high; helpers list/mark records, but no full replay worker is present.

## Message-Push Offset Commit After Failure
- What breaks: failed realtime delivery can be skipped after finite local retry.
- When to check: changing push reliability, recall/read receipt semantics, or large group fan-out.
- Inspect first: `apps/message-push/internal/consumer/consumer.go`,
  `apps/message-push/internal/consumer/realtime_handler.go`.
- Confidence: high; this is intentional to avoid one bad message blocking the partition.

## Seq-Zero Event Reliability
- What breaks: recall/read/realtime events can lack the same gap-pull fallback as normal messages.
- When to check: changing `MSG_RECALL`, `MSG_MARK_READ`, `MSG_READ_RECEIPT`, or realtime reminders.
- Inspect first: `proto/msg/msg_push_event.proto`, `apps/message-push/internal/consumer`,
  `apps/connect/internal/grpc/server.go`.
- Confidence: medium-high; client reconciliation behavior must be checked too.

## Connect ACK Conv ID Limitation
- What breaks: ACK watermarking cannot reliably record non-`MSG_PUSH` events.
- When to check: adding ack-required event types or changing envelope metadata.
- Inspect first: `apps/connect/internal/grpc/server.go`, `apps/connect/internal/svc/ack.go`.
- Confidence: high.

## Friend Apply Idempotency
- What breaks: duplicate pending friend applications under concurrency or deadline retry.
- When to check: changing friend apply create/upsert, gateway/relation retry policy,
  proto fields, or DB constraints.
- Inspect first: `apps/relation/internal/service/friend_service.go`,
  `apps/relation/internal/repository/apply_repository.go`, `model/ApplyRequest.go`,
  `proto/relation/friend_service.proto`.
- Confidence: medium; re-read current code before acting because this area is actively changing.

## Redis Retry Queue Ordering
- What breaks: cache repair tasks for the same key can execute out of order.
- When to check: changing `pkg/redisretry`, cache invalidation, or retry producer setup.
- Inspect first: `pkg/redisretry/manager.go`, `pkg/redisretry/consumer.go`.
- Confidence: medium-high.
