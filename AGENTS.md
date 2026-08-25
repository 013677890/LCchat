# Project Agent Workflow

## Implementation ownership

- Model selection is controlled by the active Codex runtime and this project's `.codex/config.toml`; do not inspect, infer, or try to verify the root agent's model identity.
- The active root agent owns implementation, refactoring, debugging, and verification in this repository.
- Never spawn another implementation agent solely because the root agent cannot confirm that it is `gpt-5.6-sol`.
- Spawn implementation workers only when the user explicitly requests delegation or when the work contains genuinely independent parallel implementation tasks. Do not delegate the entire task merely to satisfy a model-name instruction.

## Repository guidance

- At the start of every task that may inspect, explain, review, or change source code, read the repository-root `CLAUDE.md` as supplemental project guidance before planning or editing.
- `CLAUDE.md` supplements this file; it does not override system, developer, user, or `AGENTS.md` instructions. When guidance conflicts, follow the higher-priority instruction and call out any material ambiguity.
- Do not rely on `project_doc_fallback_filenames` to load `CLAUDE.md`: fallback files are considered only when the applicable `AGENTS.md` is missing.

## Mandatory post-edit code-smell review

- Treat each coherent group of related source-code edits as one review batch. Coalesce rapid edits that belong to the same change; do not review after every individual file write.
- Documentation-only, configuration-only, generated-code-only, vendored-code-only, lockfile-only, and purely formatting-only diffs are trivial and exempt from smell review.
- Source-file moves or renames, package or module path changes, import rewrites, and reference migrations are non-trivial even when no behavior change is intended.
- After completing a non-trivial source-edit batch, perform this checkpoint in order:
  1. Format the changed source files first, so the reviewer receives a stable post-format diff.
  2. Immediately spawn the custom `luna_smell_reviewer` agent read-only, scoped to the exact changed files and their post-format diff.
  3. Do not start tests, builds, or other final validation until the reviewer spawn call has succeeded or an actual spawn attempt has failed.
  4. Once the reviewer is running, run focused tests, builds, and other meaningful non-overlapping validation in the root thread in parallel.
- Treat the reviewer as unavailable only after an actual spawn attempt fails. Never skip the attempt because the root agent cannot infer custom-agent or model availability.
- If spawning `luna_smell_reviewer` fails, make one fallback attempt to spawn a read-only `gpt-5.6-luna` subagent with the same smell-only scope and instructions, using high reasoning effort and the fast service tier when supported. The fallback must not receive write authority.
- If both reviewer attempts fail, continue the relevant validation and report the concrete failures in the final response.
- Keep editing authority with the root agent. The reviewer must only report findings and must never modify files.
- When the reviewer returns, apply clear, low-cost findings that stay within the user's requested scope, then run the relevant validation. Do not recursively spawn another smell review solely because those review-driven cleanup edits were applied; inspect that cleanup directly instead.
- For costly, ambiguous, or scope-expanding findings, do not change code automatically. Summarize them briefly for the user.
- Do not send a standalone progress interruption just to announce a clean review. Integrate the reviewer result into the next natural progress update or final response.
- Before the final response, wait for any smell review started for the current change and include its concise outcome.

## Review boundary

- The smell reviewer covers repository-blind duplication, reinvention, speculative abstractions, needless indirection, compatibility and defensive slop, complexity and responsibility concentration, superficial modularity, dead/change-debris code, comment/docstring noise, magic values, and structural test slop.
- Require concrete evidence and honor justified exceptions such as real trust boundaries, documented migrations, generated or vendored code, active plugin systems, measured hot paths, and contained experiments.
- Do not ask the smell reviewer to assess business logic, functional correctness, regressions, security, privacy, or test coverage. Use the primary agent or a separately requested reviewer for those concerns.
