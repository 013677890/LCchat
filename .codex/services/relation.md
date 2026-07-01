# Relation Service

## Role
- Owns friend applications, friend list/sync/delete, remark/tag, blacklist,
  and account-deletion cleanup for relation-local data.

## Read First
- `apps/relation/cmd/providers.go`
- `apps/relation/internal/service/friend_service.go`
- `apps/relation/internal/service/blacklist_service.go`
- `apps/relation/internal/repository/friend_repository.go`
- `apps/relation/internal/repository/apply_repository.go`
- `apps/relation/internal/repository/blacklist_repository.go`
- `apps/relation/internal/consumer/account_deleted_consumer.go`
- `proto/relation`

## Stable Behavior
- Friend relation is mostly single-direction at the service/repository level.
- `HandleFriendApply` accept path creates bidirectional relations transactionally.
- `DeleteFriend` currently deletes only the current user's directional relation.
- `SyncFriendList` rolls latest version back by 2 seconds to reduce boundary misses.
- `GetTagList` is intentionally not implemented.
- Blacklist is single-direction; status restoration depends on previous relation status.

## Cache Contracts
- Friend cache: Redis hash keyed by user; field is peer UUID; value is JSON metadata.
- Empty friend list uses `__EMPTY__` with a short TTL.
- Blacklist cache: Redis ZSet keyed by user; member is peer UUID; score is blacklist timestamp.
- Pending friend-apply inbox cache: Redis ZSet keyed by target user.

## Consumer Behavior
- `account.deleted` consumer soft-deletes relations/applications and invalidates related caches.
- Dead-letter source: `relation-service:account.deleted`.

## Traps
- Friend application duplicate prevention has been a known risk: check current proto,
  repository constraints, and service upsert/unique-index behavior before changing retry
  policy or application creation.
- Gateway/internal gRPC retries can duplicate non-idempotent writes if a deadline expires
  while the first server attempt is still running.
