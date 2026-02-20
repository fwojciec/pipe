---
name: ralph
description: Execute one iteration of the Ralph loop - pick next open issue from milestone, implement, review, merge
argument-hint: "<milestone-name>"
allowed-tools: Bash, Read, Write, Edit, Grep, Glob, Task, Skill
---

# Ralph Iterate

Execute one task from a GitHub milestone. Designed for autonomous batch processing.

**Design principle**: Run to completion without user interaction. Exit cleanly so the loop can restart.

Issues are processed sequentially by number (ascending). No dependency tracking needed.

## Arguments

Milestone name: $ARGUMENTS

## Current State

Branch: !`git branch --show-current`
Uncommitted changes: !`git status --porcelain`

---

## Phase 1: Find Next Task

### 1.1 Get Open Issues in Milestone

```bash
gh issue list --milestone "$ARGUMENTS" --state open --json number,title --jq 'sort_by(.number)'
```

If no open issues, create `.ralph-complete` and exit immediately:

```bash
touch .ralph-complete
echo "Milestone complete - no open issues"
```

### 1.2 Take First Issue

Take the issue with the lowest number. Read its full details including comments:

```bash
gh issue view <number> --comments
```

**IMPORTANT**: Always read comments. Earlier work on related issues often leaves context comments that affect implementation choices.

---

## Phase 2: Setup Branch

### 2.1 Ensure Clean Main

```bash
git checkout main
git pull origin main
```

If there are uncommitted changes, stash them and warn:
```bash
git stash push -m "ralph-stash-$(date +%s)"
echo "WARNING: Uncommitted changes were stashed. Run 'git stash list' to recover them."
```

### 2.2 Create Feature Branch

```bash
git checkout -b <number>-<short-kebab-description>
```

Example: `12-add-anthropic-streaming`

---

## Phase 3: Implement

### 3.1 Implementation Approach

For feature issues, use TDD (RED-GREEN-REFACTOR):
1. Write a failing test first
2. Implement minimal code to pass
3. Refactor if needed
4. Repeat

For refactoring/removal issues:
1. Make the change
2. Fix compilation errors
3. Update/remove affected tests

### 3.2 Validate Frequently

```bash
make validate
```

Fix issues before proceeding.

### 3.3 Do NOT Commit During Implementation

Do not commit until Phase 5. A post-commit hook triggers a roborev review on
every commit. All implementation work stays uncommitted until review passes.

---

## Phase 4: Review

### 4.1 Pre-Review Validation

```bash
make validate
```

Must pass before review.

### 4.2 Review Staged Changes

Stage all changes, then review them using `--dirty`. Run this command exactly
once. Do NOT re-run it, do NOT wrap it with `2>&1`, `echo $?`, or any other
shell constructs. Each invocation submits a new paid review.

```bash
git add .
roborev review --dirty --wait
```

If the output is confusing, check results separately:

```bash
roborev status
roborev show <job-id>
```

### 4.3 Handle Review Results

**If PASS (no actionable findings)**: Proceed to Phase 5.

**If FAIL (actionable findings)**: Fix the findings:

```bash
roborev fix
```

Then re-validate, re-stage, and re-review:

```bash
make validate
git add .
roborev review --dirty --wait
```

If still failing after 2 fix cycles, yield (see 4.4).

### 4.4 Yield (On Persistent Review Failure)

If review cannot be resolved:

1. Comment on the issue with learnings:

```bash
gh issue comment <number> --body "$(cat <<'EOF'
## Attempt Failed - Learnings

**What was tried:**
- <brief description of approach>

**Why it failed:**
- <specific feedback from review>

**What to do differently:**
- <concrete changes for next attempt>
EOF
)"
```

2. Abandon branch and return to main:

```bash
git checkout main
git branch -D <branch-name>
git pull origin main
```

3. Exit cleanly - do NOT create `.ralph-complete`

The loop will retry this issue on the next iteration with improved context.

---

## Phase 5: Complete

### 5.1 Final Commit

```bash
git add .
git commit -m "$(cat <<'EOF'
<issue-title>

<brief description of changes>

Closes #<issue-number>

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
```

### 5.2 Merge to Main

```bash
git checkout main
git pull origin main
git merge <branch-name> --no-edit
git push origin main
git branch -d <branch-name>
```

### 5.3 Close Issue

```bash
gh issue comment <number> --body "$(cat <<'EOF'
## Completed

### Changes
- <summary of what changed>

### Validation
- `make validate` passes
- roborev review passed
EOF
)"

gh issue close <number>
```

### 5.4 Post Context to Next Issues

Check if the next issues in the milestone would benefit from context about what was just implemented:

```bash
gh issue list --milestone "$ARGUMENTS" --state open --json number,title --jq 'sort_by(.number) | .[0:3]'
```

For issues where this work provides genuinely useful context, post a brief comment:

```bash
gh issue comment <next-number> --body "Context from #<number>: <1-2 sentences about what was done and how it affects this issue>"
```

Skip if the relationship is superficial.

### 5.5 Clean Exit

Exit normally. Do NOT create `.ralph-complete` - there may be more issues.
The loop script will invoke another iteration.
