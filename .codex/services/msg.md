# Msg Service

## Role
- Owns message persistence, per-conversation seq allocation, send idempotency,
  conversation rows, recall, and read-state workflows.

## Read First
- `apps/msg/cmd/providers.go`
- `apps/msg/internal/handler/msg_handler.go`
- `apps/msg/internal/usecase/send_message_workflow.go`
- `apps/msg/internal/usecase/mark_read_workflow.go`
- `apps/msg/internal/usecase/recall_message_workflow.go`
- `apps/msg/internal/domain/message/service.go`
- `apps/msg/internal/domain/conversation/service.go`
- `apps/msg/internal/domain/*/repository_impl.go`
- `proto/msg`

## Stable Behavior
- Send permission is checked before persistence.
- P2P send rejects self-send, checks blacklist both directions, then requires friendship.
- Group send requires `group.GroupService.CheckGroupMember`.
- Msg independently consumes strict `group.cache` v2 events to maintain its local
  group membership conversation projection; message-push is not in that path.
- Missing relation/group clients are internal errors, not fail-open.
- Message ID uses ULID.
- `conv_id`:
  group -> target UUID;
  p2p -> `p2p-{sorted(uuid_a,uuid_b)}`.
- Redis allocates seq; DB unique key on sender/device/client-msg-id is idempotency fallback.
- Seq holes are possible if Redis allocation succeeds but DB insert fails.

## Conversation Behavior
- P2P unread is maintained per recipient row.
- Group unread is derived from `group_conversation.max_seq - read_seq`.
- Group sends update only the sender row plus one `group_conversation` row; they
  never fetch all members or batch-create member conversations.
- The shared group tuple `max_seq/last_msg_*/updated_at` advances only for a higher
  seq. An idempotent retry repairs only the fixed sender/shared rows, never all members.
- `conversation.status` is personal state. Group membership is separate:
  `membership_status=1` active, `2` left, while `0` is valid only for P2P.
- `group.cache` advances `group_conversation.projection_version` continuously.
  The first event must be `version=1/group_created`; membership changes, version
  advancement, and the idempotency mark share one DB transaction.
- Full group lists return only active membership in normal groups. Incremental
  lists return leave/dismiss/personal-delete records as `status=1` tombstones.
- Delete conversation is logical: status=1, clear/read seq to max, unread=0.
- P2P new messages reactivate through upsert. Group visibility is derived from
  shared `group_conversation.max_seq > clear_seq`, avoiding per-member writes.

## Recall And Read
- Recall DB status update is authoritative; Kafka `MSG_RECALL` is best-effort.
- Mark-read DB update is authoritative; self sync and read receipt events are best-effort.

## Traps
- Conversation schema/index and repository identity assumptions have drifted before.
  Check `risks/watchlist.md` before changing conversation upserts or DB dialect assumptions.
- Msg currently treats Redis and Kafka producer as required dependencies.
- `KAFKA_MSG_GROUP_MEMBERSHIP_GROUP_ID` must differ from
  `KAFKA_GROUP_CACHE_GROUP_ID`; sharing a group would randomly split events.
- There is no legacy membership fallback or schema dual-read. This repository uses
  the empty-database baseline directly, so a GROUP row with membership status 0 is invalid.
