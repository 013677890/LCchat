# Auth Service

## Role
- Owns account registration/login/password reset/change-email/delete-account.
- Owns verification codes and device token/session lifecycle.
- Consumes `profile_display_changed` to update login display redundancy.

## Read First
- `apps/auth/cmd/providers.go`
- `apps/auth/internal/service`
- `apps/auth/internal/repository`
- `apps/auth/internal/consumer/profile_display_changed_consumer.go`
- `proto/auth`

## Stable Behavior
- Register writes `user_account` and a `user_created` outbox event in one DB transaction.
- DeleteAccount verifies password, soft-deletes `user_account`, writes `account.deleted`
  outbox in the same transaction, then clears device login state.
- `FindAccountByEmail` returns `Found=false` on miss instead of a business error.
- `UpdateLoginDisplay` is the auth-side consumer target for profile nickname/avatar sync.
- Verification-code types:
  `1=register`, `2=login by code`, `3=reset password`, `4=change email`.

## Redis Behavior
- Rate-limit checks can fail open.
- Verification-code writes and login token writes are hard requirements.
- Redis retry producer/consumer only work when Redis/Kafka producer paths are installed.

## Event Consumer
- `profile_display_changed` uses `kafka.NewManualCommitConsumer`.
- The consumer now passes `outbox.NewDeadLetterSink(db, "auth-service:profile_display_changed")`.
- Exhausted processing failures can park in `dead_events` before offset commit.
