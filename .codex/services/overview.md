# Service Overview

Use this file to decide which service owns a behavior. Then open the specific
service file for read-first paths and traps.

## Ownership Map
- `apps/gateway`: HTTP entry, auth middleware, DTO/proto mapping, response aggregation.
- `apps/auth`: account lifecycle, passwords, verification codes, token/device sessions,
  auth-side login display redundancy.
- `apps/user`: profile lifecycle, profile search, QR-code token mapping, public/internal
  profile RPCs.
- `apps/relation`: friend applications, friend relations, remarks/tags, blacklist,
  relation/account cleanup.
- `apps/group`: group profile, membership, roles, join applications, group cache events.
- `apps/msg`: message persistence, seq/idempotency, conversation state, send/read/recall.
- `apps/connect`: WebSocket endpoint, connection registry, Redis routes, ACK store,
  internal push gRPC.
- `apps/message-push`: consumes `msg.push`, expands recipients, calls connect gRPC.

## Dependency Shape
- Gateway calls `auth`, `user`, `relation`, `msg`, `group`.
- Msg calls `relation` and `group` for send/recall permissions and group fan-out data.
- Message-push calls `group` and `connect`.
- Connect can call `auth.DeviceService` for status/active updates.
- Auth emits account/profile events; user/relation/auth/group consume outbox-derived Kafka topics.

## Degrade vs Hard Dependencies
- Gateway: Redis and MinIO are optional/degradable; downstream gRPC conns are required.
- Auth: process can start without Redis, but verification-code and login token paths need Redis.
- User: provider claims Redis fallback, but some repository paths still dereference Redis directly.
- Relation: Redis optional.
- Group: Redis optional; comments describe MySQL-only fallback.
- Msg: Redis and Kafka producer are required in the current provider graph.
- Connect: Redis and auth-device gRPC are optional/degradable.
- Message-push: Redis and group/connect clients are required for useful delivery.

## Process Pattern
- Providers construct dependencies.
- `Run` installs globals and starts background workers/servers.
- `Shutdown` stops servers/consumers before closing pools/connections.
- Async background work should use `pkg/async`, not raw goroutines in business paths.
