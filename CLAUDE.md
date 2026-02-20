# CLAUDE.md

Strategic guidance for LLMs working with this codebase.

## Why This Codebase Exists

**Core Problem**: Mainstream AI coding harnesses are heading toward complexity (multi-agent teams, orchestration layers) that adds friction for power users. The execution overhead of Node.js-based tools is also a bottleneck.

**Solution**: A minimal, Go-based agentic coding harness that lets a single agent shine. Fast binary, composable tools, pull-based streaming. Named after Unix pipes and Magritte's "Ceci n'est pas une pipe."

## Design Philosophy

- **Ben Johnson Standard Package Layout** - domain types in root, dependencies in subdirectories
- **Minimal harness, maximum agent** - the tool is a thin pipe between human, LLM, and filesystem
- **Pull-based streaming** - inspired by Rob Pike's lexer talk; no channels, no shared state
- **Single agent** - human is the orchestrator; the agent executes
- **Self-extending** - the tool can be used to improve itself

## Workflows

| Command | Purpose |
|---------|---------|
| `/work [issue-number]` | Pick issue, branch, implement with TDD, review, PR |
| `/ralph <milestone>` | One iteration of Ralph loop: next issue, implement, merge |
| `./ralph.sh "<milestone>"` | Autonomous loop: runs `/ralph` until milestone complete |
| `/gh-workflow` | GitHub issue/milestone/PR management |

```bash
make validate     # Quality gate - run before completing any task
./ralph.sh "v0.1" # Autonomous milestone execution
```

Issues within milestones are executed sequentially by issue number (ascending).
Create issues in the order they should be implemented.

## Code Review

[roborev](https://www.roborev.io/) handles all code review. Integrated into `/work`
and `/ralph` workflows automatically.

```bash
roborev review --wait            # Review HEAD commit, block until done
roborev review --branch --wait   # Review all branch changes
roborev fix                      # Auto-fix review findings
```

## Architecture Patterns

**Ben Johnson Pattern**:
- Root package (`pipe`): domain types and interfaces only (no external dependencies)
- Subdirectories: one per external dependency (`anthropic/`, `bubbletea/`)
- Capability packages: `command/` (bash tool), `fs/` (filesystem tools), `mock/` (testing)
- `cmd/pipe/`: wires everything together

**File Naming Convention**:
- `foo/foo.go`: shared utilities for the package
- `foo/foo_test.go`: shared test utilities (in `foo_test` package)
- Entity files: named after domain entity (`message.go`, `stream.go`)

**Sealed Interfaces**: `Message`, `ContentBlock`, and `Event` use unexported marker
methods to prevent external implementations. Type switches should be exhaustive.

When uncertain about where code belongs, use the `go-standard-package-layout` skill.

## Test Philosophy

**TDD is mandatory** - write failing tests first, then implement.

**Package Convention**:
- All tests MUST use external test packages: `package pipe_test` (not `package pipe`)
- This enforces testing through the public API only
- Linter (`testpackage`) will fail on tests in the same package

**Parallel Tests**:
- All tests MUST call `t.Parallel()` at the start of:
  - Every top-level test function
  - Every subtest (`t.Run` callback)
- Linter (`paralleltest`) will fail on missing parallel calls

**Assertions**:
- Use `require` for setup (fails fast)
- Use `assert` for test assertions (continues on failure)

**Interface Compliance Checks**:
Go's `var _ Interface = (*Type)(nil)` pattern verifies interface implementation at
compile time. These checks MUST be in production code, NOT in tests.

## Linting

golangci-lint enforces:
- No global state (`gochecknoglobals`) - per Ben Johnson pattern
- Separate test packages (`testpackage`)
- Error checking (`errcheck`) - all errors must be handled
- HTTP body close (`bodyclose`) - critical for streaming

## Skills

| Skill | Use when |
|-------|----------|
| `work` | Interactive single-issue development |
| `ralph` | Autonomous iteration within Ralph loop |
| `gh-workflow` | Any GitHub issue/milestone/PR management |
| `go-standard-package-layout` | Deciding where code belongs |

## Reference Documentation

- `docs/plans/` - Design documents and implementation plans
