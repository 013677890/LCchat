# Testing Map

Use this to pick verification commands after edits.

## Broad Checks
- `go test ./...`: broadest backend check; may require generated code and local deps depending on tests.
- `make proto`: regenerate proto output after editing `proto/**`.
- `make tidy`: only when module dependencies change.

## Focused Service Checks
- Gateway service/router changes: `go test ./apps/gateway/...`.
- Auth changes: `go test ./apps/auth/...`.
- User changes: `go test ./apps/user/...`.
- Relation changes: `go test ./apps/relation/...`.
- Group changes: `go test ./apps/group/...`.
- Msg changes: `go test ./apps/msg/...`.
- Connect changes: `go test ./apps/connect/...`.
- Message-push changes: `go test ./apps/message-push/...`.
- Shared packages: run the touched package tests plus dependents when practical.

## Generation
- Proto source: `proto/**`.
- Generated outputs:
  `apps/auth/pb`, `apps/relation/pb`, `apps/user/pb`, `apps/group/pb`,
  `apps/connect/pb`, `apps/msg/pb`, `pkg/commonpb`, `pkg/realtimepb`.
- Buf config: `buf.yaml`, `buf.gen.yaml`.
- Do not hand-edit generated `.pb.go` or `.pb.validate.go` files.

## External Dependency Hints
- Unit tests should be preferred for repository-independent logic.
- DB/Redis/Kafka behavior often needs compose or mocks.
- Consumer behavior changes should cover:
  handler success,
  retryable failure,
  permanent decode skip,
  panic handling,
  dead-letter parking success/failure when applicable.

## Before Commit
- Check staged set: `git diff --cached --name-only`.
- Check unstaged user work is not accidentally staged: `git status --short`.
- For docs-only `.codex` edits, a test run is not always necessary; still inspect staged diff.
