# TUI Visual Design

Minimal/geometric aesthetic for pipe's terminal UI. Tinted background blocks,
dense status bar, ASCII branding, smart information hiding.

## Design Principles

- **Content-first**: Assistant text dominates, everything else recedes
- **Tinted blocks**: Background colors differentiate message types (no borders)
- **ANSI semantic**: All colors via ANSI indices, terminal theme determines RGB
- **Comfort for long sessions**: Coherent primitives, not visual noise
- **Progressive disclosure**: Collapsed by default, expand on demand

## Block Visual System

### Theme Extensions

New fields in `Theme` for block backgrounds:

| Field | ANSI | Purpose |
|-------|------|---------|
| `UserBg` | 4 (blue) | User message block background |
| `ToolCallBg` | 3 (yellow) | Tool call block background |
| `ToolResultBg` | 8 (bright black) | Tool result background |
| `ErrorBg` | 1 (red) | Error block background |

Background colors at ANSI indices 0-7 are inherently muted in most terminal
themes. The intent is subtle tints, not saturated blocks.

### Block Rendering

**UserMessageBlock**: Full-width background tint. Bold text. No `> ` prefix —
the background tint IS the visual indicator. Padding: 1 space left.

**AssistantTextBlock**: No background. Clean markdown rendering. This is the
dominant content — it breathes.

**ThinkingBlock**: Collapsed by default. Faint text. Header: `▶ Thinking`.
No background.

**ToolCallBlock**: Background tint. Collapsed by default. Header shows tool
name. Expanded shows full args in muted style.

**ToolResultBlock**: Background tint. **Collapsed by default** (changed from
current expanded). Shows tool name + status indicator (✓/✗) + first-line
summary when collapsed. Error results expand automatically.

Summary format when collapsed:
- `bash ✓  echo hello`
- `read ✓  src/main.go`
- `write ✓  src/main.go (42 lines)`
- `bash ✗  make test (exit 1)`

`✓` in Success color, `✗` in Error color.

**ErrorBlock**: Error background tint. Always expanded.

### Spacing

Single blank line between blocks. No blank line between consecutive tool call
+ tool result pairs (they form a visual unit).

### Collapsible Blocks

All collapsible blocks (thinking, tool call, tool result) participate in
Tab/Shift+Tab focus cycling. Same navigation model as current.

## Status Bar

Dense, single-line, Pi-inspired. Muted style throughout.

### Layout

```
~/code/pipe (main)                              claude-opus-4-5
```

Left: working directory + git branch.
Right: model name.

When running:
```
~/code/pipe (main) ● Generating...              claude-opus-4-5
```

`●` in accent color for activity indication.

### Future Metrics

Status bar is designed for future token/cost tracking:
```
~/code/pipe (main) ↑1.2k ↓800 R12k W3k $0.42 14.2%    claude-opus-4-5
```

Not in scope for this implementation — will be added when token tracking exists.

### Separators

Thin horizontal rules (`─` box-drawing, full width, muted) above and below
the status bar:

```
viewport content...
────────────────────────────────────────────────
~/code/pipe (main)                    claude-opus-4-5
────────────────────────────────────────────────
input area
```

## Welcome Screen / Branding

On empty session (no messages), show centered FIGlet-style ASCII art in accent
color:

```
         _
   _ __ (_)_ __   ___
  | '_ \| | '_ \ / _ \
  | |_) | | |_) |  __/
  | .__/|_| .__/ \___|
  |_|     |_|

  Ceci n'est pas une pipe.
```

Disappears when first message is sent. The tagline references Magritte's
painting, connecting to the project name.

## Information Hiding Defaults

| Block | Default State | Rationale |
|-------|---------------|-----------|
| User Message | Expanded | Always visible |
| Assistant Text | Expanded | Primary content |
| Thinking | Collapsed | Internal reasoning |
| Tool Call | Collapsed | Show name only |
| Tool Result | Collapsed | Show name + status summary |
| Tool Result (error) | Expanded | Errors must be visible |
| Error | Expanded | Must be visible |

## Implementation Notes

### ToolResultBlock Changes

ToolResultBlock becomes collapsible (like ThinkingBlock/ToolCallBlock):
- Add `collapsed bool` field
- Implement `ToggleMsg` handling in `Update`
- Generate summary line from content (tool name + ✓/✗ + first line)
- Participate in focus cycling (`updateBlockFocus` / `cycleFocusPrev`)

### New Styles

New `Styles` fields for block backgrounds:
- `UserBg lipgloss.Style` — background only
- `ToolCallBg lipgloss.Style` — background only
- `ToolResultBg lipgloss.Style` — background only
- `ErrorBg lipgloss.Style` — background only

Block `View` methods apply background style to entire rendered content using
`lipgloss.NewStyle().Background(color).Width(width).Render(content)`.

### Status Bar

`statusLine()` method restructured:
- Compute left content (cwd + branch)
- Compute right content (model name)
- Pad middle with spaces to fill viewport width
- Apply muted style
- Add separator lines above and below

Git branch detection: shell out to `git rev-parse --abbrev-ref HEAD` at
startup, cache the result. Or use go-git if already a dependency.

### Welcome Screen

New `welcomeView(width, height int) string` method:
- Center ASCII art horizontally and vertically within viewport
- Apply accent style
- Return as viewport content when `len(m.blocks) == 0`

### Height Calculation

Status bar grows from 1 line to 3 lines (separator + status + separator).
Adjust `viewportHeight` calculation: `borderHeight` changes from 2 to 4.
