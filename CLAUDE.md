## Design Philosophy

- Ben Johnson Standard Package Layout - domain types and functional core in root, dependencies in subdirectories, /go-standard-package-layout skill if in doubt
- Elm Architecture via BubbleTea for TUI - tree of modules pattern, /bubble-tea skill if in doubt
- avoid global state
- dependency injection
- optimize for testability

## Quality Gatge

make validate

## Test Philosophy

- write failing tests first, then implement
- prefer behavioral assertions to testing implementation
- all tests with t.Parallel (prevents data races)
- all tests in *_test packages (forces testing via public apis only)
