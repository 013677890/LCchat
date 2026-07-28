# Outbox And Consumers

Use this when changing account/profile/group events, manual Kafka consumers,
dead-letter behavior, or idempotency.

## Topics
- Redis retry: `redis-retry-queue`.
- User created: `user_created`.
- Profile display changed: `profile_display_changed`.
- Account deleted: `account.deleted`.
- Message push: `msg.push`.
- Group projections: `group.cache` is consumed independently by the group Redis
  projector and the msg membership projector.
- `group.cache` is created with a fixed **3 partitions**. Kafka key is
  `group_uuid` (`outbox.entity_id`). Same group is always ordered; different
  groups may run in parallel across partitions.
- Each of group/msg starts N independent manual-commit Readers in the same
  consumer group (`pkg/kafka.ManualConsumerPool`). Defaults:
  `KAFKA_GROUP_CACHE_PROJECTOR_CONCURRENCY=3`,
  `KAFKA_MSG_GROUP_MEMBERSHIP_PROJECTOR_CONCURRENCY=3`.
  Explicit values must be integers in 1..64 or startup fails.
  Worker count above partition count only yields idle workers.
  Do not alter partitions online from the app.

## Outbox Contract
- Business transactions insert `outbox_events`.
- Debezium/Kafka Connect routes records by `event_type`.
- Event key is `entity_id`; for group cache this is `group_uuid`.
- Current Outbox JSON events are delivered as a top-level JSON object via
  `table.expand.json.payload=true`. `group.cache` and `msg.push` deliberately
  reject JSON strings, Debezium-style wrappers, unknown fields, and old schemas.

## Manual Consumer Contract
- `pkg/kafka.NewManualCommitConsumer` commits offset only after handler success.
- Decode/invalid-payload errors use `kafka.Permanent`; with a configured sink they
  are written to `dead_events` before the offset is committed.
- Processing errors return non-nil so the same message is retried.
- A configured `DeadLetterSink` lets the consumer park a message after retry budget
  exhaustion and then commit offset.
- If dead-letter parking fails, offset is not committed and the partition remains blocked.

## Current Dead-Letter Behavior
- Table/model: `dead_events` / `pkg/outbox.DeadEvent`.
- Status values: `pending`, `replayed`, `discarded`.
- Schema lives in `config/mysql/001_schema.sql` and `scripts/migration/004_dead_events.sql`.
- Sources currently configured:
  - `auth-service:profile_display_changed`
  - `user-service:user_created`
  - `user-service:account.deleted`
  - `relation-service:account.deleted`
  - `group-service:group.cache`
  - `msg-service:group-membership`

## Retry/Timeout Defaults
- Manual consumer per-attempt timeout default: 10s.
- Manual consumer retry budget default: 2m when a sink is configured.
- Error backoff remains per consumer config, commonly 1s.
- MySQL driver socket timeout defaults are added by `pkg/mysql` if DSN omits them:
  `timeout=5s`, `readTimeout=10s`, `writeTimeout=10s`.

## Idempotency Caveat
- `idempotent_events` unique `(event_type,event_id)` prevents many duplicates.
- The common pattern is still `Check -> business -> Mark`.
- That pattern is not true exactly-once for non-idempotent business side effects unless
  the business write and `MarkIdempotent` happen in one transaction.
- Msg's `group.cache` membership projector does put its business updates,
  per-group version advancement, and `MarkIdempotent` in one MySQL transaction.
