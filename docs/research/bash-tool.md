# Building bash tools for AI coding agents: the 2026 state of the art

**The bash tool is the single most important tool in any agentic coding harness.** Every major implementation — Claude Code, OpenAI Codex, Aider, OpenCode, Goose, SWE-agent, Cline, Amp — converges on the same insight: a well-built shell executor paired with a file-edit tool can replace dozens of specialized tools while achieving better results. Vercel proved this dramatically when stripping 80% of custom tools from their d0 agent and replacing them with a single bash tool — success rates jumped from 80% to **100%**, latency dropped 5×, and token usage fell by half. But the implementation details matter enormously. Output handling, security sandboxing, process lifecycle, and failure-mode resilience separate production-grade implementations from toys. This report synthesizes the architecture decisions, source code patterns, and hard-won lessons from every major open-source implementation as of February 2026.

---

## Architecture patterns have converged on two competing models

Two fundamental design patterns dominate bash tool implementations: **formal tool-use API** and **text-pattern extraction**. The vast majority of production tools now use the formal approach.

**The formal tool-use model** (Claude Code, OpenAI Codex, OpenCode, Goose, Cline, Amp) registers the bash tool as a structured function with typed parameters. Claude Code defines its tool as `bash_20250124` with parameters `command` (string), `timeout` (milliseconds, max 600,000), `description` (5–10 word purpose), and `restart` (boolean to restart the session). OpenAI Codex exposes **four distinct shell tool variants**: `shell` (array-based, passed directly to `execvp`), `shell_command` (string-based, wrapped in user's shell), `exec_command` (PTY-based with persistent sessions), and `write_stdin` (sends input to existing sessions). Different models get different variants — GPT-5 gets the array-based `shell`, while models with the UnifiedExec feature get `exec_command` plus `write_stdin` for interactive use.

**The text-pattern model** (Aider, mini-SWE-agent) skips tool-use APIs entirely. Aider parses LLM output text for shell command suggestions and presents them with a Y/N confirmation prompt. Mini-SWE-agent constrains the LLM to output exactly one bash code block, which the harness extracts with regex and executes via `subprocess.run`. This is simpler but loses structured error handling and makes output routing harder.

**Persistent vs. stateless shells** represent another key architectural split. Claude Code, OpenCode, and Codex maintain **persistent bash sessions** where working directory and shell state carry across commands. Claude Code's reference implementation spawns a single `subprocess.Popen(["/bin/bash"])` with pipes for stdin/stdout/stderr, reusing it for the entire session. OpenCode uses a `PersistentShell` struct in Go. By contrast, mini-SWE-agent deliberately uses stateless `subprocess.run` — every command is independent, which is simpler but loses state. The persistent approach wins for real coding tasks where `cd`, environment activation, and shell variables matter, but the stateless approach is more robust to crashes and easier to sandbox.

**PTY vs. pipe-based execution** is a critical low-level choice. OpenCode uses pseudo-terminal (PTY) execution via `pty.ts`, which enables capturing output from programs that behave differently when not connected to a terminal (color codes, progress bars, interactive prompts). OpenAI Codex's `exec_command` variant also uses PTY allocation. However, Claude Code deliberately uses **pipe-based execution without PTY support** — a limitation documented in issue #9881. Cline takes a unique approach by hooking into VS Code's `terminal.shellIntegration.executeCommand()` API, reading output from the `execution.read()` async iterator. This gives it terminal-like behavior within the IDE context but introduces race conditions in output retrieval timing.

---

## Output handling is the hardest problem, and seven strategies exist

Managing command output is the defining challenge of bash tool design. A single `find /` or `cat large_file.log` can produce megabytes of text, and naively stuffing it into the context window destroys agent performance. Seven distinct strategies have emerged, often used in combination.

**Strategy 1: Middle truncation with character limits.** Claude Code's approach is the most widely copied. It sets a **default maximum of 30,000 characters** (configurable via `BASH_MAX_OUTPUT_LENGTH`), then applies **middle-truncation** — preserving the beginning and end of output while cutting the middle. When truncation fires, a marker like `"... [480572 characters truncated] ..."` is inserted. The rationale: command beginnings contain startup info and headers; endings contain results, exit codes, and final errors. The middle is usually repetitive data. This is simple, deterministic, and handles most cases well.

**Strategy 2: Head+tail line-based truncation.** OpenAI Codex originally used **256 lines or 10 KiB** (whichever hit first), showing the first 128 and last 128 lines. This approach has a documented flaw: **line counts don't correlate with token usage**. Two hundred fifty-six short lines might be 1–2K tokens, while 100 long lines could be 10K+. GitHub issue #6426 on the Codex repo documented how this forces excessive tool calls and hides critical context in the middle (build errors, scattered test failures). The community pushed for token-based limits instead.

**Strategy 3: Token-aware truncation and budgeting.** The more sophisticated approach counts actual tokens rather than characters or lines. OpenCode triggers output compaction when accumulated tool output exceeds **20,000 tokens**, with a protected context window of **40,000 tokens**. Tools with `status === "completed"` become candidates for pruning; protected tools (like `skill`) are never pruned. The context budgeting pattern looks like allocating fixed percentages of the context window across layers — system prompt, memory, docs, tools, history, task — and compressing any layer that exceeds its budget.

**Strategy 4: Conversation compaction and summarization.** When the context window fills, Claude Code runs a **summarization pass** that preserves architectural decisions and recent work while discarding redundant tool outputs. A lighter variant called **tool result clearing** strips old tool call/result pairs from message history. OpenCode assigns compacted tools a `state.time.compacted` timestamp. Goose offers a `/compact` slash command for on-demand conversation summarization. SWE-agent takes a targeted approach: observations preceding the last 5 are collapsed into single lines, and all past error messages except the first are omitted.

**Strategy 5: Filesystem offloading.** Manus (acquired by Meta) writes old tool results to files, applying summarization only when offloading reaches diminishing returns. Cursor Agent similarly offloads tool results and trajectories to the filesystem, letting the agent read back selectively. Anthropic's context engineering guide explicitly recommends this: rather than loading all data upfront, use `head`, `tail`, and `grep` to analyze large data without loading full objects into context. This treats the **filesystem as extended working memory**.

**Strategy 6: Memory pointers for zero-loss context management.** A November 2025 paper by Labate et al. ("Solving Context Window Overflow in AI Agents") introduces a novel approach: store large tool outputs outside the context window entirely, and let the model interact using **short identifier tokens (pointers)**. Each tool gets a wrapper that resolves input pointers, executes the original tool, stores large outputs externally, and returns only a pointer. This achieved **7× fewer tokens** than traditional workflows in materials science benchmarks with no information loss — unlike truncation or summarization, which are inherently lossy.

**Strategy 7: Bash-native context retrieval.** Vercel's approach, documented in their "bash is all you need" series, flips the paradigm entirely. Instead of feeding large outputs into the prompt, keep data in the filesystem and let the agent use `grep`, `cat`, `find`, `jq`, `head`, `tail` to retrieve **small targeted slices** on demand. The agent decides what context it needs using Unix tools it already knows from training data. Their AI SDK 6 extends this with code execution tools that chain operations programmatically, **keeping intermediate results entirely out of context** and sending only final results back.

---

## Security requires both filesystem and network isolation

Claude Code's security team learned a critical lesson: **filesystem isolation alone is insufficient, and network isolation alone is insufficient**. Without network controls, an agent can exfiltrate SSH keys. Without filesystem controls, it can backdoor system resources. Both layers are mandatory.

**OS-level sandboxing** is the gold standard. Claude Code uses **bubblewrap on Linux and seatbelt on macOS** — OS-level primitives, not containers. The sandbox restricts read/write to the current working directory and subdirectories, with read-only access elsewhere and denied access to sensitive directories. This is open-sourced as the `sandbox-runtime` npm package. OpenAI Codex uses **Landlock + seccomp on Linux and Seatbelt on macOS**, with three modes: `read-only`, `workspace-write`, and `danger-full-access`. Codex sets `CODEX_SANDBOX_NETWORK_DISABLED=1` in sandboxed environments.

**Container and VM isolation** provides stronger boundaries. SWE-agent runs commands inside Docker containers via the SWE-ReX backend, achieving full isolation from the host. Google Cloud's Agent Sandbox on GKE uses **gVisor** (user-space kernel with syscall interception) with warm pod pools for fast provisioning. For the strongest isolation, **microVMs** like Firecracker or Kata Containers provide a dedicated kernel per workload with 100–125ms cold starts. Standard Docker containers share the host kernel and are insufficient for untrusted AI-generated code — container escape vulnerabilities like CVE-2024-21626 demonstrate the risk.

**Vercel's just-bash takes a radical approach**: reimplementing bash entirely in TypeScript. No shell process is spawned, no arbitrary binary execution is possible. It includes reimplementations of `awk`, `grep`, `sed`, `jq`, `cat`, `ls`, `find`, `head`, `tail`, `sort`, `wc`, `xargs`, and 30+ other commands. An OverlayFS layer means writes stay in memory and are discarded. Network access is disabled by default with URL prefix allow-lists. The tradeoff: it cannot run real binaries like `node` or `python`.

**Permission models** vary in sophistication. OpenCode uses **tree-sitter to parse bash commands** and extract permission patterns for compound commands — the most sophisticated static analysis approach. Permission rules use glob-style matching (`git *: allow`, `rm *: ask`). Claude Code uses prefix-matching permission rules (`Bash(git diff:*)`, `Bash(npm run test:*)`) with deny-overrides-allow precedence. Goose uses `.gooseignore` files and blocks **31 sensitive environment variables** via a hardcoded `DISALLOWED_KEYS` list. Amp uses a simple allowlist in VS Code settings — and suffered a vulnerability in July 2025 where agents could modify their own config to allowlist commands (a prompt injection vector now patched).

**NVIDIA's AI Red Team** identifies four mandatory controls for agentic shell execution: network egress controls to prevent data exfiltration, blocking file writes outside the workspace, protecting dotfiles and config directories (`.bashrc`, `.zshrc` can enable code execution in different security contexts), and securing hooks, MCP servers, and instruction files like `CLAUDE.md` that could give attackers durable ways to shape agent behavior.

---

## Performance bottlenecks center on shell spawning and caching

Shell spawning overhead is a documented production concern across multiple tools. Claude Code has open bugs reporting **30-second hangs on macOS** (#25016), **200-second delays on Windows** (#4049), and bash commands running "extremely slowly" on Linux where trivial commands take tens of seconds (#10181). The root causes trace to how the harness launches and manages bash processes internally, with behavioral differences between CLI (Node.js subprocess) and Desktop (Electron) implementations.

**Persistent shell sessions** are the primary mitigation. Anthropic's reference implementation maintains a single `Popen` process with `bufsize=0` for immediate I/O, using queue-based readers for stdout and stderr with configurable timeouts (default 10 seconds for output collection). OpenCode's `PersistentShell` in Go maintains shell state across commands. The key insight: **amortize process creation cost across many commands** rather than spawning a new shell per invocation.

**Prompt caching** is critical for cost control. Manus identified **cache hit rate** as their "most important metric" for production agents. A higher-capacity model with caching can be cheaper than a lower-cost model without it. Coding agents like Claude Code would be "cost-prohibitive without caching." Context mutations — including tool output insertion — must consider cache invalidation. Every time bash output modifies the conversation history, previously cached prefixes may be invalidated.

**Background execution** addresses long-running commands. Claude Code supports auto-backgrounding: commands that exceed the timeout get backgrounded rather than killed, with `BashOutput` and `KillShell` tools for managing background processes. Users can press Ctrl+B to explicitly background commands. Background tasks write output to temporary files that the agent can check later, preventing blocking of the main agent loop.

---

## Twelve failure modes that separate good implementations from bad

The gap between a demo bash tool and a production one comes down to handling edge cases. These are the most impactful failure modes documented across implementations.

**Interactive command detection** is the most common failure. Commands like `vim`, `git rebase -i`, `npm init`, and Python REPLs spawn interactive sessions that hang the agent indefinitely. Claude Code's system prompt explicitly forbids interactive git flags: "NEVER use git commands with the -i flag." The only real solutions are timeout enforcement (Claude Code defaults to **120 seconds**, configurable up to 10 minutes) or PTY-based interactive support (which only Codex's `exec_command` and Google Gemini CLI v0.9.0 fully implement).

**Stderr handling** trips up many implementations. Common pitfalls: many CLI tools write important information to stderr (progress indicators, warnings), not just errors. Claude Code captures both stdout and stderr and returns them to the model. Its hook system uses exit codes as control flow — exit 0 means proceed, exit 2 means blocking error with stderr fed back to the model. **The critical pattern**: always capture and return both streams with the exit code. OpenAI Codex returns structured `{stdout, stderr, exitCode}` objects. Cline's VS Code terminal integration combines both streams (the terminal API doesn't separate them), which is simpler but loses the distinction.

**Encoding issues** cause silent data corruption. OpenAI's implementation decodes output with `errors="ignore"`: `stdout_bytes.decode("utf-8", errors="ignore")`. This prevents crashes from non-UTF-8 binary output but silently drops characters. Production implementations must handle binary output from commands like `cat` on non-text files.

**Pre-commit hook recovery** is a subtle gotcha. Claude Code's prompt explicitly instructs: "If the commit fails due to pre-commit hook changes, retry the commit ONCE to include automated changes." Without this, agents get stuck in infinite loops or give up when hooks auto-format code, creating a mismatch between the committed content and the post-hook state.

**Environment variable non-persistence** surprises many implementers. Despite Claude Code maintaining a persistent bash session, **environment variables do NOT persist** between commands. Each command runs in a fresh shell environment. Workarounds include the `CLAUDE_ENV_FILE` mechanism (a script sourced before each command), activating environments before starting the agent, or using SessionStart hooks.

Other documented failure modes include: sandbox permission errors requiring retry with `sandbox=false`, file paths with spaces needing double quotes, shell snapshot creation failures when `/usr/bin/env bash` can't be found (platform-specific path issues), nil pointer dereferences in OpenCode's `PersistentShell.Exec` at line 272, and Cline's intermittent output retrieval failures from race conditions in the VS Code `read()` API timing.

---

## What the best implementations actually do differently

Examining the source code reveals that the highest-quality implementations share several distinguishing patterns that go beyond basic shell execution.

**Claude Code** invests heavily in **context engineering around the bash tool**. Its system prompt steers the model away from bash for operations that have dedicated tools: "avoid `find`, `grep`, `cat`, `head`, `tail`, `ls` in favor of built-in Read, Grep, Glob, LS tools." This reduces unnecessary context consumption from verbose bash output. The sub-agent architecture spawns isolated agents with fresh context windows for complex tasks, each returning a condensed **1–2K token summary** — preventing any single bash-heavy subtask from poisoning the main context. The sandbox reduces permission prompts by **84%** in Anthropic's internal usage, dramatically improving agent flow.

**OpenAI Codex** has the most sophisticated shell abstraction layer, with a `ShellType` enum supporting Bash, Zsh, Sh, PowerShell, Cmd, Fish, and Nushell. The `derive_exec_args` function transforms commands per shell type (e.g., `"echo hello"` becomes `["/bin/bash", "-lc", "echo hello"]` on Unix, `["powershell.exe", "-Command", "echo hello"]` on Windows). Shell detection follows a hierarchy: user-specified → `getpwuid` → `which` → fallback paths → `/bin/sh`. Model-specific tool selection means different models get different shell interfaces based on their capabilities.

**OpenCode** stands out for using **tree-sitter to statically analyze bash commands** before execution. This lets it extract permission patterns from compound commands (pipes, `&&` chains) and check each component against glob-style permission rules. This is significantly more sophisticated than Claude Code's prefix matching or Cline's simple `requires_approval` boolean.

**SWE-agent** pioneered the **Agent-Computer Interface (ACI) philosophy**: "Environment feedback should be informative but concise." It provides explicit messages for empty outputs ("Your command ran successfully and did not produce any output"), validates bash syntax before execution (`BashIncorrectSyntaxError`), and enforces that file edits only apply if they don't produce linter errors. The `max_consecutive_execution_timeouts` counter exits the agent entirely after too many hangs — a circuit breaker that prevents runaway resource consumption.

**Goose** differentiates through its **MCP-native architecture**. All tools communicate via Model Context Protocol, with name-prefixing to prevent collisions (`developer__shell`). Its recipe system adds retry logic with validation — shell command checks, file existence checks, and content regex checks can verify that a multi-step operation succeeded before proceeding.

---

## Conclusion: the bash-first paradigm and what comes next

The strongest signal from this research is the **convergence toward bash-minimalism**. Sean Goedecke's October 2025 observation captures the trend: "The current direction is extremely minimal tooling — execute shell command + make a patch edit." Guillermo Rauch put it more directly: "The fundamental coding agent abstraction is the CLI." Vercel's results validated this empirically.

Three key insights emerge for anyone building a bash tool today. First, **output handling must be token-aware, not line-aware**, and the most promising approaches avoid putting large outputs in context at all — using filesystem offloading, memory pointers, or letting the agent retrieve targeted slices with Unix tools. Second, **security requires defense in depth**: OS-level sandboxing (not just permission prompts), network egress control, filesystem write restriction, and protection of configuration files that could enable persistent compromise. Third, **persistent shell sessions with robust timeout and background execution** are table stakes for production use, but environment variable management remains an unsolved ergonomic problem across all implementations.

The frontier is moving toward eliminating the shell process entirely (Vercel's TypeScript bash interpreter), using code execution to keep intermediate results out of context (AI SDK 6's programmatic tool chaining), and treating the filesystem as the primary context management layer rather than the conversation history. The bash tool that wins will be the one that gives the agent maximum capability while consuming minimum context.
