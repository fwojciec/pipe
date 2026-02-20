# Standard Package Layout Refactor Plan

## Context

This plan documents the refactor required to bring `github.com/fwojciec/pipe` into strict compliance with Ben Johnson's Standard Package Layout as defined in `~/.claude/skills/go-standard-package-layout/SKILL.md`.

The current codebase is already close to the target model:
- Domain types and interfaces are in the root package.
- Provider and UI implementations are in dependency-named subpackages (`anthropic/`, `bubbletea/`, `json/`).
- `cmd/pipe/main.go` acts as composition root.

The remaining gaps are mostly structural and naming consistency issues.

## Refactor Goals

1. Achieve full package-layout compliance with dependency/capability naming.
2. Keep runtime behavior unchanged.
3. Preserve test coverage and keep `go test ./...` green at each phase.
4. Make composition explicit in `cmd/pipe` for tool wiring.

## Non-Goals

1. No user-visible feature changes.
2. No behavioral changes in tool semantics, streaming, persistence, or TUI interactions.
3. No provider expansion (Anthropic remains the only provider implementation in this refactor).

## Compliance Gaps to Close

1. `builtin/` is concept-layer packaging and mixes multiple dependencies in one package.
2. `json/json.go` is a monolith instead of entity-oriented files.
3. `mock/provider.go` combines multiple domain mock entities.
4. Root file organization can be made strictly entity-oriented (`provider/request/stream`, `role/stop_reason/usage`).
5. Missing compile-time interface assertion for `bubbletea.Model` as `tea.Model`.

## Target End State

### Package Layout

```text
pipe/
├── errors.go
├── event.go
├── message.go
├── provider.go
├── request.go
├── role.go
├── session.go
├── stop_reason.go
├── stream.go
├── tool.go
├── usage.go
├── agent/
│   └── loop.go
├── anthropic/
│   ├── anthropic.go
│   ├── client.go
│   └── stream.go
├── bubbletea/
│   ├── bubbletea.go
│   └── model.go
├── exec/
│   └── bash.go
├── fs/
│   ├── edit.go
│   ├── glob.go
│   ├── grep.go
│   ├── read.go
│   └── write.go
├── json/
│   ├── json.go
│   ├── session.go
│   ├── message.go
│   ├── content_block.go
│   └── usage.go
├── mock/
│   ├── doc.go
│   ├── provider.go
│   ├── stream.go
│   └── tool_executor.go
└── cmd/pipe/
    ├── main.go
    └── tools.go
```

### Architectural Rules After Refactor

1. Root package contains only domain vocabulary (types, interfaces, validation rules).
2. Implementation packages are dependency/capability named and import root.
3. No concept-layer dispatcher package for tools; tool wiring is explicit in `cmd/pipe`.
4. One implementation entity per file where applicable.
5. Compile-time interface assertions exist for all concrete interface implementations.

## Implementation Plan

## Phase 0: Safety Baseline

### Changes

1. Record baseline test state.
2. Snapshot package graph and imports before moves.

### Commands

```bash
go test ./...
go list ./...
```

### Exit Criteria

1. Baseline is green before refactor starts.

## Phase 1: Root Package File Normalization

### Changes

1. Split `provider.go` into:
- `stream.go`: `StreamState`, `Stream`.
- `provider.go`: `Provider`.
- `request.go`: `Request`, `Validate()`.

2. Split `usage.go` into:
- `role.go`: `Role` constants.
- `stop_reason.go`: `StopReason` constants.
- `usage.go`: `Usage` struct and comments.

3. Keep API signatures unchanged.

### Files

1. `provider.go` (modified)
2. `request.go` (new)
3. `stream.go` (new)
4. `usage.go` (modified)
5. `role.go` (new)
6. `stop_reason.go` (new)

### Exit Criteria

1. No exported API behavior change.
2. `go test ./...` passes.

## Phase 2: Tool Implementation Decomposition (`builtin` -> `exec` + `fs`)

### Rationale

`builtin/` currently combines unrelated dependencies and responsibilities. For strict layout compliance:
- command execution logic belongs in `exec/` (identity: `os/exec`)
- filesystem tooling belongs in `fs/` (identity: filesystem operations)

### Changes

1. Create `exec/bash.go` by moving logic from `builtin/bash.go`.
2. Create `fs/read.go`, `fs/write.go`, `fs/edit.go`, `fs/grep.go`, `fs/glob.go` from corresponding `builtin/*.go` files.
3. Move shared result helpers currently in `builtin/builtin.go` into package-local helpers in `exec` and `fs` (or small duplicated helpers if clearer).
4. Move corresponding tests:
- `builtin/bash_test.go` -> `exec/bash_test.go`
- `builtin/read_test.go` -> `fs/read_test.go`
- `builtin/write_test.go` -> `fs/write_test.go`
- `builtin/edit_test.go` -> `fs/edit_test.go`
- `builtin/grep_test.go` -> `fs/grep_test.go`
- `builtin/glob_test.go` -> `fs/glob_test.go`

### Files

1. `exec/bash.go` (new)
2. `exec/bash_test.go` (new)
3. `fs/read.go` (new)
4. `fs/write.go` (new)
5. `fs/edit.go` (new)
6. `fs/grep.go` (new)
7. `fs/glob.go` (new)
8. `fs/read_test.go` (new)
9. `fs/write_test.go` (new)
10. `fs/edit_test.go` (new)
11. `fs/grep_test.go` (new)
12. `fs/glob_test.go` (new)
13. `builtin/bash.go`, `builtin/bash_test.go`, `builtin/read.go`, `builtin/read_test.go`, `builtin/write.go`, `builtin/write_test.go`, `builtin/edit.go`, `builtin/edit_test.go`, `builtin/grep.go`, `builtin/grep_test.go`, `builtin/glob.go`, `builtin/glob_test.go`, `builtin/builtin.go` (removed after migration — `builtin/executor.go` and `builtin/executor_test.go` remain until Phase 3)

### Exit Criteria

1. No logic regression in tool behavior.
2. `go test ./exec ./fs` passes.
3. `go test ./...` passes.

## Phase 3: Move Tool Dispatch Into Composition Root

### Rationale

`cmd/pipe` should explicitly wire dependencies. Current `builtin.Executor` is a reusable concept-layer dispatcher package.

### Changes

1. Add `cmd/pipe/tools.go` containing:
- registry of tool definitions (`[]pipe.Tool`)
- dispatch implementation of `pipe.ToolExecutor` bound to concrete handlers from `exec` and `fs`

2. Update `cmd/pipe/main.go` to use local wiring from `cmd/pipe/tools.go`.
3. Remove `builtin/executor.go` and `builtin/executor_test.go`.

### Files

1. `cmd/pipe/tools.go` (new)
2. `cmd/pipe/main.go` (modified)
3. `builtin/executor.go` (removed)
4. `builtin/executor_test.go` (removed)

### Exit Criteria

1. Tool registration and execution remain equivalent.
2. `cmd/pipe` remains explicit composition root.
3. `go test ./cmd/pipe ./...` passes.

## Phase 4: JSON Package Entity Split

### Rationale

`json/json.go` currently mixes session persistence, DTOs, and conversion logic.

### Changes

1. Keep `json/json.go` for package-wide DTO declarations and shared constants/helpers only.
2. Create:
- `json/session.go`: `Save`, `Load`, `MarshalSession`, `UnmarshalSession`
- `json/message.go`: `marshalMessage`, `unmarshalMessage`
- `json/content_block.go`: content block marshal/unmarshal
- `json/usage.go`: usage DTO conversion helpers

3. Keep external API surface unchanged.

### Files

1. `json/json.go` (modified)
2. `json/session.go` (new)
3. `json/message.go` (new)
4. `json/content_block.go` (new)
5. `json/usage.go` (new)
6. `json/json_test.go` (adjust imports/references if needed)

### Exit Criteria

1. JSON wire format remains identical.
2. All existing JSON tests pass unchanged in assertions.

## Phase 5: Mock Package Entity Split

### Changes

1. Add `mock/doc.go` package docs.
2. Split `mock/provider.go` into:
- `mock/provider.go`: `Provider`
- `mock/stream.go`: `Stream`

3. Rename `mock/tool.go` to `mock/tool_executor.go`.
4. Move compile-time interface assertions (`var _ pipe.X = ...`) from `mock/mock_test.go` into the production file for each concrete type (e.g., `mock/provider.go`, `mock/stream.go`, `mock/tool_executor.go`).

### Files

1. `mock/doc.go` (new)
2. `mock/provider.go` (modified)
3. `mock/stream.go` (new)
4. `mock/tool_executor.go` (new)
5. `mock/tool.go` (removed)
6. `mock/mock_test.go` (updated references only as needed)

### Exit Criteria

1. Mock behavior remains identical.
2. `go test ./mock` passes.

## Phase 6: Interface Assertions and Test Structure Cleanup

### Changes

1. Add compile-time check in `bubbletea/model.go`:
- `var _ tea.Model = Model{}`

2. Optional strict cleanup for package-named test helper files:
- move reusable helpers into `anthropic/anthropic_test.go`
- move reusable helpers into `bubbletea/bubbletea_test.go`

### Files

1. `bubbletea/model.go` (modified)
2. `anthropic/anthropic_test.go` (new, if helper extraction is done)
3. `bubbletea/bubbletea_test.go` (new, if helper extraction is done)

### Exit Criteria

1. Interface contracts are compile-time verified.
2. `go test ./anthropic ./bubbletea` passes.

## Phase 7: Delete Legacy Package and Final Cleanup

### Changes

1. Remove now-empty `builtin/` directory.
2. Update any stale imports and comments referencing `builtin`.
3. Update docs mentioning old layout.

### Files

1. `README.md` (if package references are present later)
2. `docs/plans/2026-02-18-domain-types-design.md` (optional addendum note)
3. any remaining files importing `github.com/fwojciec/pipe/builtin`

### Exit Criteria

1. `rg -n 'pipe/builtin|package builtin'` returns no results.
2. `go test ./...` passes.

## API Compatibility and Release Notes

This refactor changes import paths for tool implementations and removes `builtin` package APIs.

Expected breaking changes:
1. `github.com/fwojciec/pipe/builtin` package removal.
2. `builtin.NewExecutor()` removal.
3. Tool function imports move to `exec` and `fs` packages.

If compatibility is required for a transition window, a temporary deprecated `builtin` shim can be added and removed in a subsequent major release. Full strict compliance requires complete removal of the concept-layer `builtin` package.

## Validation Matrix

1. Unit tests: `go test ./...`
2. Build command: `go build ./cmd/pipe`
3. Package graph check: `go list ./...`
4. Compliance grep checks:

```bash
rg -n 'package builtin|pipe/builtin'
rg -n '^var _ ' --glob '*.go'
```

## Rollout Strategy

1. Execute phases in order with a green test suite after each phase.
2. Prefer small commits per phase to simplify review and revertability.
3. Run full test suite before and after directory/package deletions.

## Definition of Done

1. No concept-layer `builtin` package remains.
2. Tool implementations are split into dependency/capability-aligned packages.
3. Composition-root wiring for tools is explicit in `cmd/pipe`.
4. `json` and `mock` follow entity-oriented file organization.
5. Root package is strictly domain-only and entity-oriented by file.
6. All tests pass with no behavior regressions.
