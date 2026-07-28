# Group Service

## Role
- Owns group profile, membership, roles, join applications, internal member checks,
  member-id lookup, and Redis cache projection events.

## Read First
- `apps/group/cmd/providers.go`
- `apps/group/internal/service/group_service.go`
- `apps/group/internal/service/group_join_service.go`
- `apps/group/internal/repository/group_repository.go`
- `apps/group/internal/repository/group_event_mapper.go`
- `apps/group/internal/consumer/cache_projector.go`
- `proto/group/group_service.proto`

## Stable Behavior
- Create group inserts owner as active member with role `2`.
- Request member UUIDs are trimmed, de-duplicated, and exclude owner.
- Permission matrix:
  dismiss owner only; name/avatar/notice admin or owner; add_mode owner only;
  transfer owner owner only; role changes owner only; add members admin or owner.
- Owner cannot leave directly.
- Group dismissal changes group status; it does not bulk-delete member rows.
- Removed/quit members are soft-deleted and can be restored by add/approve.
- `CheckGroupMember` verifies group status before trusting membership cache.

## Join Applications
- `add_mode=0`: direct join.
- `add_mode=1`: pending application.
- Duplicate pending application returns already-exists semantics.
- Review action: `1=approve`, `2=reject`; only admin/owner can review.

## Cache Projector
- `group.cache` is a Redis projector, not a second business service.
- Events are emitted by `group_event_mapper` inside the same DB transaction as group writes.
- Projector validates minimal payload shape and projects final snapshots into Redis.
- Dead-letter source: `group-service:group.cache`.
- Uses `pkg/kafka.ManualConsumerPool`: N independent Readers (default 3 via
  `KAFKA_GROUP_CACHE_PROJECTOR_CONCURRENCY`), same consumer group, Kafka assigns
  partitions. Different partitions parallel; same partition serial; same
  `group_uuid` ordered by key. Topic fixed at 3 partitions.

## Traps
- Projector must not re-run permission checks; it consumes committed facts.
- Incremental cache events generally patch-if-exists, especially user group lists.
