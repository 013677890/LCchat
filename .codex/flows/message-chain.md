# Message Chain

Use this when changing send, recall, read, push delivery, or WebSocket ACK behavior.

## Main Send Path
1. Gateway accepts HTTP JSON and forwards to msg gRPC.
2. Msg handler takes identity/device from auth context metadata.
3. Send workflow checks relation/group permission.
4. Message domain creates message with idempotency and seq.
5. Conversation domain upserts sender/receiver/group conversation state.
6. Msg publishes `MsgPushEvent` to Kafka `msg.push`.
7. Message-push consumes, resolves routes/members, calls connect gRPC.
8. Connect writes protobuf envelope to WebSocket.
9. Client ACK updates Redis ACK watermark for `MSG_PUSH`.

## Important Files
- Gateway: `apps/gateway/internal/router`, `apps/gateway/internal/service`.
- Msg handler: `apps/msg/internal/handler/msg_handler.go`.
- Send workflow: `apps/msg/internal/usecase/send_message_workflow.go`.
- Message domain: `apps/msg/internal/domain/message`.
- Conversation domain: `apps/msg/internal/domain/conversation`.
- Push consumer: `apps/message-push/internal/consumer/consumer.go`.
- Connect gRPC: `apps/connect/internal/grpc/server.go`.
- Connect ACK: `apps/connect/internal/svc/ack.go`.

## Recall
- DB message status update is authoritative.
- Kafka `MSG_RECALL` is best-effort.
- History pull still reveals recalled state even if realtime delivery fails.

## Mark Read
- DB read-state update is authoritative.
- `MSG_MARK_READ` syncs same user's other devices.
- `MSG_READ_RECEIPT` notifies the peer side separately.
- Publish failures do not roll back DB read state.

## Reliability Notes
- `MSG_PUSH` has seq and can be recovered by client gap pull.
- `MSG_RECALL`, read events, and realtime reminders are weaker unless the client has
  an explicit reconciliation path.
