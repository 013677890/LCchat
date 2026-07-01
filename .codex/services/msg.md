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
- Delete conversation is logical: status=1, clear/read seq to max, unread=0.
- New messages can reactivate the row through upsert.

## Recall And Read
- Recall DB status update is authoritative; Kafka `MSG_RECALL` is best-effort.
- Mark-read DB update is authoritative; self sync and read receipt events are best-effort.

## Traps
- Conversation schema/index and repository identity assumptions have drifted before.
  Check `risks/watchlist.md` before changing conversation upserts or DB dialect assumptions.
- Msg currently treats Redis and Kafka producer as required dependencies.
