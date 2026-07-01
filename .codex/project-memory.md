# LCChat Codex Memory

This directory uses progressive disclosure. Start here, then open only the
small topic file that matches the work in front of you.

## How To Read
- 30 seconds: read this file.
- 2 minutes: open `services/overview.md` plus the one service file you will edit.
- 5 minutes: add the relevant flow, risk, or ops file.
- Source of truth is still code. Memory points to contracts, traps, and files to read first.

## Navigation
- `services/overview.md`: service ownership and dependency boundaries.
- `services/gateway.md`: HTTP aggregation surface and downstream clients.
- `services/auth.md`: accounts, login, devices, auth-side events.
- `services/user.md`: profiles, search, QR tokens, profile consumers.
- `services/relation.md`: friends, applications, blacklist, account cleanup.
- `services/group.md`: group write/read behavior and cache projector.
- `services/msg.md`: message domain, conversation state, send/read/recall workflows.
- `services/connect.md`: WebSocket access, routes, ACKs, push gRPC.
- `services/message-push.md`: `msg.push` fan-out and realtime delivery behavior.
- `flows/message-chain.md`: send/recall/read delivery chain.
- `flows/outbox-and-consumers.md`: outbox, Kafka consumers, dead letters, idempotency.
- `ops/runbook.md`: startup, env, migrations, CDC, proto/build commands.
- `ops/testing-map.md`: focused test/build/generation commands.
- `risks/watchlist.md`: known traps to check before changing behavior.
- `memory-roadmap.md`: what has been split and what remains.

## Durable Repo Facts
- Module: `github.com/013677890/LCchat-Backend`.
- Stack: Go 1.25, gRPC, Gin, GORM, Redis, Kafka, Wire, MinIO, Docker Compose.
- Architecture: multi-service IM backend; each app uses `cmd/main.go`, Wire providers,
  `App.Run(ctx)`, and explicit shutdown.
- Gateway is HTTP entry only; business rules belong downstream.
- `relation` is a standalone service. Do not assume friend/blacklist logic lives in `user`.
- `group` is a real write service, not a read-only skeleton.
- `message-push`, not `connect`, consumes Kafka `msg.push`.
- `connect` is socket access plus route/active-state/push-gRPC handling.

## Current High-Value Facts
- Main message path:
  `gateway -> msg gRPC -> msg domain/usecase -> Kafka msg.push -> message-push -> connect gRPC -> WebSocket`.
- Outbox path:
  business DB transaction writes `outbox_events`; Debezium/Kafka Connect routes by `event_type`.
- Consumer-side dedupe uses `idempotent_events`, but `Check -> business -> Mark`
  is not atomic unless the business handler wraps it that way.
- Manual Kafka consumers can now be configured with per-attempt timeout, retry budget,
  and `DeadLetterSink`; the current auth/user/relation/group projector consumers park
  exhausted messages in `dead_events`.
- `message-push` remains a custom finite local retry consumer. It has a per-attempt
  timeout, then commits offset after retries fail; it does not currently use `dead_events`.
- MySQL DSNs should include driver socket timeouts (`timeout=5s&readTimeout=10s&writeTimeout=10s`)
  or rely on `pkg/mysql` to add defaults.

## Read Order For Code Changes
1. `apps/<service>/cmd/{main,app,providers}.go`
2. `apps/<service>/internal/{handler,service,usecase,repository}`
3. `proto/<service>`
4. Matching `.codex/services/*.md` and `.codex/flows/*.md`
5. Docs under `doc/` only for intent; some are stale.

## Do Not Assume
- Do not assume old docs are current on service ownership.
- Do not assume Redis is optional everywhere; `msg` and `message-push` still require it.
- Do not edit generated `apps/*/pb` files by hand; regenerate proto output.
- Do not rely on `.codex` as proof. Re-read the pointed code before changing behavior.
