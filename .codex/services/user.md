# User Service

## Role
- Owns profile read/update/search, avatar URL, QR-code token mapping, batch profile lookup,
  and internal profile RPCs.
- Consumes `user_created` and `account.deleted`.

## Read First
- `apps/user/cmd/providers.go`
- `apps/user/internal/service`
- `apps/user/internal/repository`
- `apps/user/internal/consumer/user_created_consumer.go`
- `apps/user/internal/consumer/account_deleted_consumer.go`
- `proto/user`

## Stable Behavior
- `CreateProfile` is idempotent and is called by the `user_created` consumer.
- `UpdateProfile` only updates non-empty/non-zero fields; empty string cannot clear
  signature/birthday through the current API.
- Birthday must be a real `YYYY-MM-DD` date.
- Profile/avatar updates write `profile_display_changed` outbox in the same DB transaction.
- Profile cache invalidation failure is sent to Redis retry.
- QR tokens are reused while the user-token mapping exists; mappings use 48h TTL.

## Consumer Behavior
- `user_created` dead-letter source: `user-service:user_created`.
- `account.deleted` dead-letter source: `user-service:account.deleted`.
- Both use manual commit and `dead_events` when retry budget is exhausted.

## Traps
- Providers describe MySQL-only Redis fallback, but profile read/batch/QR repository paths
  have historically dereferenced Redis directly. Verify nil Redis behavior before claiming
  user-service can run fully without Redis.
- `apps/user` still has a legacy/proto-local `user.GroupService`; current msg/message-push
  use `apps/group` instead.
