---
name: work
description: Pick a GitHub issue, create branch, implement using TDD, review, and create PR
argument-hint: "[issue-number]"
allowed-tools: Bash, Read, Write, Edit, Grep, Glob, Task, Skill, AskUserQuestion
---

# Work on GitHub Issue

End-to-end workflow: select issue -> create branch -> implement -> review -> PR.

**Design principle**: Run to completion with minimal user interaction. Only stop for:
- Pre-flight failures (not on main, dirty working tree)
- Task selection (if no issue number provided)
- Blocking review feedback (persistent FAIL verdicts)
- Post-PR merge decision

## Current State

Branch: !`git branch --show-current`
Uncommitted changes: !`git status --porcelain`

## Arguments

$ARGUMENTS

---

## Phase 1: Setup

### 1.1 Pre-flight Validation

Before proceeding, verify:
- [ ] Currently on `main` branch (if not, ask user before proceeding)
- [ ] Working tree is clean (if not, ask user how to proceed)

### 1.2 Task Selection

**If issue number provided in $ARGUMENTS**: Use that issue.

**If no arguments**: List open issues and let user choose:

```bash
gh issue list --state open --json number,title,labels --limit 20
```

Use AskUserQuestion to let user pick which issue to work on.

### 1.3 Branch Setup

1. Get issue details:
   ```bash
   gh issue view <number> --json number,title
   ```

2. Create branch: `<number>-<short-kebab-description>`
   ```bash
   git checkout -b <branch-name>
   ```

3. Read full issue **including comments**:
   ```bash
   gh issue view <number> --comments
   ```

   **IMPORTANT**: Always read comments. Earlier work often leaves context.

---

## Phase 2: Implementation

### 2.1 TDD Workflow

Follow RED-GREEN-REFACTOR:
1. Write a failing test first
2. Implement minimal code to pass
3. Refactor if needed
4. Repeat

**TDD applies to**: Functions with logic, modules with behavior, integration points, error handling.

**TDD does NOT apply to**: Type definitions, data structures without behavior, configuration, boilerplate wiring.

### 2.2 Validate During Development

Run frequently:
```bash
make validate
```

### 2.3 Do NOT Commit During Implementation

Do not commit until Phase 4. A post-commit hook triggers a roborev review on
every commit. All implementation work stays uncommitted until review passes.

---

## Phase 3: Review

### 3.1 Pre-Review Validation

```bash
make validate
```

### 3.2 Review Staged Changes

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

### 3.3 Handle Review Results

**If PASS (no actionable findings)**: Proceed to Phase 4. Do not stop to ask.

**If FAIL (actionable findings)**: Fix the findings:

```bash
roborev fix
make validate
git add .
roborev review --dirty --wait
```

If still failing after 2 fix cycles, stop and present blocking issues to the user.

---

## Phase 4: Finish

### 4.1 Commit and Validate

```bash
git add .
git commit -m "$(cat <<'EOF'
<issue-title>

<brief description of changes>

Closes #<issue-number>

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
EOF
)"
make validate
```

### 4.2 Create Pull Request

```bash
git push -u origin <branch-name>

gh pr create --title "<title>" --body "$(cat <<'EOF'
## Summary
<2-3 bullets of what changed>

Closes #<issue-number>

## Test Plan
- [ ] `make validate` passes
- [ ] roborev review passed

Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

### 4.3 Final Status

Post completion comment on the issue:

```bash
gh issue comment <number> --body "$(cat <<'EOF'
## Implementation Complete

PR: <pr-url>

### Summary
<brief description>

### Changes
- <key changes>
EOF
)"
```

Report the PR URL to the user.

### 4.4 Await Merge

Ask the user what to do next using AskUserQuestion:

1. **"Merge it"** - Merge and clean up:
   ```bash
   gh pr merge <pr-number> --squash --delete-branch
   git checkout main
   git pull origin main
   ```

2. **"I'll merge it myself"** - Return to main:
   ```bash
   git checkout main
   git pull origin main
   ```

3. **"Keep working"** - Stay on branch for more changes.
