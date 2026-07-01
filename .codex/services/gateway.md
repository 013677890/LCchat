# Gateway Service

## Role
- HTTP entry only. It maps JSON/HTTP requests to downstream gRPC calls and aggregates
  profile/relation/group/message responses.
- It should not re-own downstream business rules.

## Read First
- `apps/gateway/cmd/providers.go`
- `apps/gateway/internal/router`
- `apps/gateway/internal/service`
- `apps/gateway/internal/pb/client_connection.go`

## Current Surface
- Message HTTP APIs include send, pull, get-by-ids, recall, conversation list,
  mark-read, delete conversation, and conversation settings.
- Group HTTP APIs include create/dismiss/info/update, notice, owner transfer,
  member role update, join application workflow, add/remove member, member lists,
  and user group lists.
- Friend/blacklist/application APIs mostly aggregate relation facts with user profile cards.

## Aggregation Conventions
- Identity comes from auth context, not request body, for owner/from/current user fields.
- Batch profile filling should de-duplicate UUIDs and use batches of 100 where existing code does.
- Profile fill failures often degrade; relation facts should still return when possible.
- Search split:
  email keyword -> auth internal account lookup -> user batch profile;
  non-email keyword -> user profile search -> relation batch friendship fill.

## Traps
- Retry config on gRPC clients must use exact protobuf service names.
- Gateway retry policies can affect non-idempotent downstream writes; check
  `risks/watchlist.md` before widening retryable methods or status codes.
