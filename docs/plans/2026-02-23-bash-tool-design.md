# Bash Tool Design

World-class bash tool for an LLM-driven coding agent. Informed by research on
Claude Code, OpenAI Codex, OpenCode, pi-mono, SWE-agent, and Vercel's bash-first
architecture.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Shell model | Stateless (fresh bash -c per command) | Crash-proof, simple. LLM chains `cd && cmd`. Pi-mono validates this works. |
| Output truncation | Tail, last 2000 lines or 50KB | Errors/results at the end. Two independent limits, whichever hits first. |
| Large output | Filesystem offloading to temp file | Full output to `/tmp/pipe-bash-*.log`. LLM told where to find it. |
| Timeout behavior | Auto-background on timeout | Process continues, output written to file, LLM notified with pid. |
| Stderr handling | Separate stdout/stderr | Structured `{stdout, stderr, exitCode}` returned to LLM. |
| Output sanitization | Strip ANSI + control chars | Clean tokens for the LLM. Keep tab/newline/CR, drop rest. |
| Security | None (local dev machine) | Trusted environment. Architect for future sandboxing if needed. |
| PTY support | None | LLM doesn't need terminal rendering. Pipe-based is cleaner. |

## Execution Model

Stateless execution with fixed CWD from session start. Each command:

1. Spawn `bash -c <command>` with session CWD
2. Capture stdout and stderr to separate rolling buffers
3. On completion: sanitize → truncate → offload if needed → return
4. On timeout: detach process, return partial output with pid

Process group isolation via `Setpgid: true`. Kill entire tree with
`syscall.Kill(-pid, SIGKILL)`.

## Output Pipeline

```
raw stdout/stderr
    → rolling buffer (100KB cap, keeps tail)
    → sanitize (strip ANSI, filter control chars)
    → tail truncate (last 2000 lines or 50KB)
    → if truncated: write full output to temp file
    → format result with truncation notice
```

### Sanitization

1. Strip ANSI escape codes: CSI sequences (`\x1b[...`), OSC sequences (`\x1b]...`)
2. Filter control characters: keep `\t` (0x09), `\n` (0x0A), `\r` (0x0D); drop
   all other bytes ≤ 0x1F
3. Normalize line endings: `\r\n` → `\n`

### Tail Truncation

Two independent limits — whichever hits first:

- **2000 lines** maximum
- **50 KB** (51,200 bytes) maximum

Algorithm works backwards from end of output, collecting complete lines until a
limit is hit. Edge case: if a single line exceeds 50KB, take the tail of that
line.

Returns `TruncateResult` with: total lines, total bytes, output lines, output
bytes, whether truncated, which limit triggered, whether last line is partial.

### Filesystem Offloading

File offloading triggers **only when total bytes exceed 50KB** (the rolling buffer
threshold). Line-only truncation (e.g., 3000 short lines totaling 20KB) does NOT
create a temp file — the truncated output is small enough for the context window.

When offloading triggers:

1. Write full untruncated output to `/tmp/pipe-bash-<random-hex>.log`
2. Append actionable notice to truncated output:
   `[stdout: Showing last 2000 of 8500 lines. Full output: /tmp/pipe-bash-a1b2c3d4.log]`
3. If file I/O fails, notice warns: `Full output file may be incomplete: <path> (<error>)`

The LLM uses `read` or `grep` tools to access full output selectively.

### Rolling Buffer

During command execution, buffer up to 100KB (2x truncation limit). When buffer
exceeds 100KB, discard oldest data. This bounds memory for commands that produce
unbounded output.

## Background Execution

### Auto-Backgrounding

When a command exceeds its timeout (default 120s):

1. Detach the process from the bash tool's goroutine
2. Spawn a goroutine that continues reading output to the temp file
3. Return immediately with partial output + background notice:

```
[Command backgrounded after 120s timeout (pid 12345). Partial output:

stdout:
<last N lines>

stderr:
<last N lines>

Full output: /tmp/pipe-bash-a1b2c3d4.log
Use check_pid or kill_pid to manage.]
```

### Background Process Management

A session-scoped `map[int]*BackgroundProcess` tracks active background processes.

`BackgroundProcess` holds:
- `*exec.Cmd` — the running process
- Output temp file path
- Goroutine that reads stdout/stderr to file
- Last read offset (for incremental reads)

The LLM manages background processes via the same `bash` tool using mutually
exclusive parameters: `command` runs a new command, `check_pid` checks on a
background process, `kill_pid` terminates one.

## Tool Schema

```json
{
  "name": "bash",
  "description": "Execute a bash command. Output truncated to last 2000 lines or 50KB; if truncated, full output saved to temp file. Commands exceeding timeout are auto-backgrounded.",
  "parameters": {
    "type": "object",
    "properties": {
      "command": { "type": "string", "description": "The bash command to execute" },
      "timeout": { "type": "integer", "description": "Timeout in ms before auto-backgrounding (default: 120000)" },
      "check_pid": { "type": "integer", "description": "Check on a backgrounded process, return new output" },
      "kill_pid": { "type": "integer", "description": "Kill a backgrounded process, return final output" }
    }
  }
}
```

## Result Format

### Normal completion

```
stdout:
<sanitized, truncated stdout>

stderr:
<sanitized, truncated stderr>

exit code: 0
```

With truncation notice appended if output was truncated.

### Error (non-zero exit)

Same format but `ToolResult.IsError = true`.

### Backgrounded

```
[Command backgrounded after 120s (pid 12345).

stdout (last 500 lines):
...

stderr (last 100 lines):
...

Full output: /tmp/pipe-bash-a1b2c3d4.log]
```

### check_pid result

```
[Process 12345 still running.

New output since last check (200 lines):
...

Full output: /tmp/pipe-bash-a1b2c3d4.log]
```

Or if finished:

```
[Process 12345 exited with code 0.

stdout (last 2000 lines):
...

stderr:
...

Full output: /tmp/pipe-bash-a1b2c3d4.log]
```

## Package Structure

Following Ben Johnson Standard Package Layout. All in `exec` package:

| File | Contents |
|------|----------|
| `bash.go` | Tool definition, `ExecuteBash()`, argument parsing, result formatting |
| `sanitize.go` | `Sanitize()` — ANSI stripping, control char filtering |
| `truncate.go` | `TruncateTail()` — tail truncation with line/byte limits |
| `background.go` | `BackgroundProcess`, `BackgroundRegistry` (session-scoped map) |
| `*_test.go` | Corresponding test files |

### Dependency Injection

```go
// Commander abstracts process execution for testing.
type Commander interface {
    Command(ctx context.Context, name string, args ...string) *exec.Cmd
}
```

Production uses `os/exec`. Tests inject a mock that returns controlled output.

## What We're Not Doing

- **Persistent shell sessions** — stateless is simpler and pi-mono proves it works
- **PTY support** — LLM doesn't need terminal emulation
- **Token-aware truncation** — byte/line limits are simpler and sufficient
- **OS-level sandboxing** — local dev machine, trusted environment
- **Permission model** — no command allowlists/denylists for now
- **Tree-sitter command parsing** — overkill for trusted environment
- **Interactive command detection** — rely on timeout + auto-background
