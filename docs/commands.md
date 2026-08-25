---
title: Command reference
nav_order: 3
---

# Command reference

Commands are repository-local by default. `clean` and `prune` accept `--global`
to discover Git repositories under common development roots and manage their
worktrees in one view.

A global flag `--no-color` is available on every command.

## Daily driving

### `bonsai list`

Every worktree with branch, age, last commit, ahead/behind status, and PR
state.

```bash
bonsai list
bonsai list --no-pr      # only worktrees with no PR
bonsai list --offline    # skip GitHub lookups (faster)
bonsai list --json
```

```text
  #   PATH                              BRANCH                AGE    LAST COMMIT              +/-      PR
  ──────────────────────────────────────────────────────────────────────────────────────────────────────
      ~/projects/myapp                  main                  2h     chore: bump deps         +0/-0    -
  1   .claude/worktrees/feat-auth       feat/auth             3d     add OAuth flow           +4/-0    open
  2   .claude/worktrees/fix-payments    fix/payments          21d    fix stripe webhook       +0/-0    merged
  3   .claude/worktrees/feat-dashboard  feat/dashboard        8d     WIP: new dashboard       +2/-0    none
```

### `bonsai new <branch>`

Create a worktree and branch from the configured base branch.

```bash
bonsai new feat/search
bonsai new fix/login --base develop
bonsai new spike/idea --open      # open in $EDITOR afterwards
```

### `bonsai push [branch-or-path]`

Push a worktree's branch, optionally open a PR, optionally remove the worktree
afterwards.

```bash
bonsai push
bonsai push feat/search
bonsai push --pr
bonsai push --web                          # open PR creation in the browser
bonsai push --pr --remove
bonsai push --pr --remove --keep-history   # removal stays recoverable
bonsai push --dry-run
```

`--remove` asks before deleting the worktree. Add `--yes` only when that
removal has already been approved in an automated workflow.

### `bonsai switch`

Fuzzy picker that prints a `cd` command for the worktree you choose.

### `bonsai open <n>`

Open a worktree in your editor by its `bonsai list` number.

```bash
bonsai open 2
bonsai open 2 --editor code
```

## Cleanup

### `bonsai clean`

Interactive picker for merged, stale, or otherwise removable worktrees.

```bash
bonsai clean
bonsai clean --all                                # not just cleanup candidates
bonsai clean --global --all
bonsai clean --global --claude                    # only .claude/worktrees
bonsai clean --global --claude --keep-history
bonsai clean --stale 7
bonsai clean --force                              # allow selecting review/protected
bonsai clean --offline
bonsai clean --root ~/workspace
```

Picker keys: `up`/`down` or `j`/`k` move, `space` toggles the highlighted row,
`a` selects all safe rows, `n` clears the selection, `enter` opens a final
review screen. Confirm there with `y`, or go back with `n`/`esc`.

### `bonsai prune`

Classify merged or inactive worktrees as `safe`, `review`, or `protected`.
Automatic cleanup only ever removes `safe` worktrees and their local branches.

```bash
bonsai prune
bonsai prune --dry-run
bonsai prune --claude
bonsai prune --global --claude --dry-run
bonsai prune --global --claude --json
bonsai prune --global --claude --keep-history --json
bonsai prune --global --root ~/workspace --dry-run
bonsai prune --apply <plan-id> --yes
bonsai prune -y                    # safe worktrees only, no prompts
bonsai prune --stale 7
bonsai prune --offline
```

`--json` saves a plan for 15 minutes and prints it. See
[the plan contract]({% link agents.md %}#the-plan-contract) for the fields and
the apply rules.

Global scans are bounded to development directories that already exist:
`~/Github`, `~/GitHub`, `~/Projects`, `~/Developer`, `~/Code`, and `~/src`.
Use repeatable `--root` flags for other locations. Repository-local
`.bonsai.toml` settings are honored independently during a global scan.

### `bonsai rm <n> [n...]`

Remove worktrees by the numbers shown in `bonsai list`.

```bash
bonsai rm 2
bonsai rm 1 3 5
bonsai rm --dry-run 2
bonsai rm --force 2            # even with unpushed commits
bonsai rm --keep-history 2
```

## Inspection

### `bonsai status`

Dashboard view of every worktree's working tree state.

```bash
bonsai status
bonsai status --json
```

### `bonsai stats`

Aggregate metrics across all worktrees.

```bash
bonsai stats
bonsai stats --offline
bonsai stats --json
```

### `bonsai doctor`

Detect broken and orphaned worktrees.

### `bonsai sync`

Bring every non-main worktree up to date with the base branch.

```bash
bonsai sync
bonsai sync --merge          # merge instead of rebase
bonsai sync --dry-run
bonsai sync --dry-run --json
```

## Archiving

### `bonsai snapshot`

Compressed tar archives of a worktree, as a safety net before deletion. Unlike
[trash entries]({% link recovery.md %}), a snapshot copies the whole worktree.

```bash
bonsai snapshot create 2
bonsai snapshot list
bonsai snapshot restore <snapshot>
```

## Structured output

For scripts, CI, and coding agents, these commands write machine-readable JSON
to stdout with no spinner or progress text:

```bash
bonsai list --json
bonsai status --json
bonsai stats --json
bonsai sync --dry-run --json
bonsai prune --global --claude --json
bonsai trash list --json
bonsai undo --json
bonsai config check --json
```

`sync --json` reports each non-main worktree as `synced`, `skipped`, or
`failed`, with a reason or error when applicable.

## Plain output

Bonsai disables ANSI colors automatically when stdout is redirected or piped.
To force plain output in a terminal, set `NO_COLOR` to any non-empty value or
pass `--no-color`:

```bash
NO_COLOR=1 bonsai list
bonsai --no-color status
```
