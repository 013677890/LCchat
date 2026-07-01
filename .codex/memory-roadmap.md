# LCChat Long-Term Memory Roadmap

## Goal
- Turn local Codex memory from a large notes file into a practical project navigation system.
- Optimize for the questions that come up when changing code, debugging production-like behavior, or onboarding back into the repo after time away.
- Keep code as source of truth. Memory should point to files, summarize stable contracts, and call out traps; it should not become a stale reimplementation of the code.

## Current State
- `project-memory.md` is now the short progressive-disclosure entrypoint.
- First-layer topic files exist under:
  - `services/`
  - `flows/`
  - `ops/`
  - `risks/`
- The remaining weakness is depth and freshness, not entry shape:
  - gateway HTTP route mapping is still not first-class yet
  - data-flow files cover message/outbox first, but not every product workflow
  - testing map is useful but still broad
  - detailed service notes should be extended only when code work proves a fact useful

## Target Memory Files
- `project-memory.md`
  - Keep as the one-screen entrypoint and navigation rule.
- `services/*.md`
  - Service ownership, read-first files, stable behavior, and service-local traps.
- `flows/*.md`
  - Cross-service flows and event chains.
- `ops/runbook.md`
  - Startup, env vars, ports, migrations, CDC, and troubleshooting.
- `ops/testing-map.md`
  - Build/test/generation commands and verification routing.
- `risks/watchlist.md`
  - Known risks, mismatches, and "check this before changing" notes.
- Future `api-map.md`
  - Gateway HTTP route to handler to downstream gRPC method.
  - Include auth requirement, timeout budget, request identity source, and aggregation behavior.
- Future flow expansions
  - Core sequence flows:
    - register -> user_created -> profile
    - login/refresh/logout/device active
    - friend apply/accept/delete/blacklist
    - group create/join/review/member changes
    - account delete cleanup

## Priority Plan
- P0 done: Split and index memory.
  - `project-memory.md` is a short entrypoint.
  - Service files exist under `services/`.
  - `risks/watchlist.md` exists with current high-risk traps.
  - `ops/runbook.md` and `ops/testing-map.md` exist.
- P0 done: Capture current dead-letter/retry facts.
  - Manual consumers with `DeadLetterSink` park to `dead_events`.
  - `message-push` remains a custom finite retry consumer and does not use `dead_events`.
  - MySQL socket timeout defaults are documented.
- P1: Build `api-map.md`.
  - Read `apps/gateway/internal/router`.
  - For every route, record method/path, handler, downstream gRPC, auth requirement, timeout, aggregation/degradation notes.
  - Keep this concise and route-oriented, not implementation-heavy.
- P1: Expand flow files.
  - Add account/profile lifecycle flow.
  - Add friend apply/accept/delete/blacklist flow.
  - Add group create/join/review/member-change flow.
- P1: Deepen service files only when code work needs it.
  - Prefer stable contracts and read-first pointers over line-by-line summaries.
- P2: Add generated `api-map.md`.
  - Keep route-oriented and compact.

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
- Memory files are structured so the first read is small, and deeper files are opened only on demand.
