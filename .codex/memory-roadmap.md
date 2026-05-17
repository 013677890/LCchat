# LCChat Long-Term Memory Roadmap

## Goal
- Turn local Codex memory from a large notes file into a practical project navigation system.
- Optimize for the questions that come up when changing code, debugging production-like behavior, or onboarding back into the repo after time away.
- Keep code as source of truth. Memory should point to files, summarize stable contracts, and call out traps; it should not become a stale reimplementation of the code.

## Current State
- `project-memory.md` is already rich on service boundaries, message flow, group/relation/user/auth/connect details, Redis keys, event flows, and known implementation truths.
- The main weakness is shape, not volume:
  - too many different kinds of knowledge live in one file
  - operational commands and API maps are not first-class yet
  - known risks are present but scattered
  - gateway HTTP route mapping is not complete enough for fast feature work

## Target Memory Files
- `project-memory.md`
  - Keep as the high-level architecture and service-boundary overview.
  - Keep only the most durable code truths and pointers to specialized memory files.
- `watchlist.md`
  - Known risks, mismatches, and "check this before changing" notes.
  - Each item should include impact, trigger condition, likely files, and confidence.
- `runbook.md`
  - How to start services, required dependencies, env vars, ports, build/test/proto commands, and common failure checks.
  - Include Debezium/Kafka Connect assumptions because outbox depends on them.
- `api-map.md`
  - Gateway HTTP route to handler to downstream gRPC method.
  - Include auth requirement, timeout budget, request identity source, and aggregation behavior.
- `data-flow.md`
  - Core sequence flows:
    - register -> user_created -> profile
    - login/refresh/logout/device active
    - send message -> msg.push -> message-push -> connect -> WebSocket ACK
    - friend apply/accept/delete/blacklist
    - group create/join/review/member changes
    - account delete cleanup
- `testing-map.md`
  - Test/build/generation commands, test scope by service, and external dependencies needed by tests.
  - Include proto generation and generated-code expectations.

## Priority Plan
- P0: Split and index memory.
  - Add `watchlist.md` with the currently known sharp edges.
  - Add `runbook.md` skeleton from providers/env/docker-compose.
  - Add short navigation links in `project-memory.md`.
- P0: Complete high-risk watchlist.
  - `user_repository.go` Redis nil dereferences despite provider saying MySQL-only fallback.
  - `conversation` schema unique key is `(owner_uuid,target_uuid)` while repository upsert references `(owner_uuid,conv_id)`.
  - msg seq allocation can produce gaps because Redis INCR happens before DB insert succeeds.
  - message-push stage-1 commits offsets even after finite local retry failure.
  - connect ACK watermark currently records conv_id only for `MSG_PUSH`.
- P1: Build `api-map.md`.
  - Read `apps/gateway/internal/router`.
  - For every route, record method/path, handler, downstream gRPC, auth requirement, timeout, aggregation/degradation notes.
  - Keep this concise and route-oriented, not implementation-heavy.
- P1: Build `runbook.md`.
  - Read `docker-compose.yml`, `deploy/env/chatserver.env.example`, app providers, and any Makefile/scripts.
  - Record startup dependency order and the minimum viable local stack.
  - Record ports, required envs, and common "service starts but feature fails" checks.
- P1: Build `testing-map.md`.
  - Read Makefile/scripts and common generated-code layout.
  - Record commands already proven useful and commands that require MySQL/Redis/Kafka.
  - Record which generated `pb` paths are ignored and should not be hand-edited.
- P2: Build `data-flow.md`.
  - Convert the highest-value flows into compact sequence notes or Mermaid diagrams.
  - Prefer file references and event names over long prose.
- P2: Trim `project-memory.md`.
  - After specialized files exist, remove duplicated operational/API/watchlist detail from the main memory.
  - Keep `project-memory.md` below a size that can be skimmed quickly.

## Writing Rules
- Prefer code-derived facts over docs.
- Prefer stable contracts over line-by-line implementation notes.
- Every risk item should answer: "What breaks?", "When does it break?", "Where do I inspect first?"
- Every operational note should include exact file/env/command references.
- Do not record secrets from real env files; use only example values or variable names.
- Mark uncertainty explicitly with "verify before changing" instead of presenting guesses as facts.

## Definition Of Done
- A future Codex session can answer these quickly from `.codex` memory:
  - Which service owns this behavior?
  - Which route/RPC do I touch?
  - Which envs and dependencies are required to run it?
  - Which tests or generation commands should I run?
  - What known traps should I check before editing?
- Memory files stay local-only via `.gitignore` and do not pollute normal project commits.
