# bonsai

`bonsai` is a friendly CLI for managing git worktrees.

If AI coding tools keep spawning branches and worktrees all over a repo, bonsai helps keep things tidy. It shows what exists, what is stale, what has a PR, what still has unpushed work, and what is safe to clean up.

## Why bonsai?

`git worktree` is powerful, but the day-to-day experience is still pretty manual:

- hard to see everything at a glance
- easy to forget stale worktrees
- annoying to tell what has been pushed or merged
- no quick cleanup flow

`bonsai` fixes that with a small CLI that feels made for real branch-heavy workflows.

## Install

```bash
go install github.com/sauravpanda/bonsai@latest
```

Or from source:

```bash
git clone https://github.com/sauravpanda/bonsai
cd bonsai
make install
```

Requirements:

- Go 1.21+
- Optional: [GitHub CLI](https://cli.github.com/) for PR status and PR creation

## Quick Start

```bash
bonsai new feat/search
bonsai list
bonsai push --pr --remove
bonsai clean
```

Typical flow:

1. Create a worktree for a task.
2. See all active worktrees in one place.
3. Push and open a PR when the work is ready.
4. Clean up merged or stale worktrees without guesswork.

## Core Commands

### `bonsai list`

See every worktree with branch, age, last commit, ahead/behind status, and PR state.

```bash
bonsai list
bonsai list --no-pr
bonsai list --offline
```

Example output:

```text
  #   PATH                              BRANCH                AGE    LAST COMMIT              +/-      PR
  ──────────────────────────────────────────────────────────────────────────────────────────────────────
      ~/projects/myapp                  main                  2h     chore: bump deps         +0/-0    -
  1   .claude/worktrees/feat-auth       feat/auth             3d     add OAuth flow           +4/-0    open
  2   .claude/worktrees/fix-payments    fix/payments          21d    fix stripe webhook       +0/-0    merged
  3   .claude/worktrees/feat-dashboard  feat/dashboard        8d     WIP: new dashboard       +2/-0    none
```

### `bonsai new <branch>`

Create a new worktree and branch from the configured base branch.

```bash
bonsai new feat/search
bonsai new fix/login --base develop
bonsai new spike/idea --open
```

### `bonsai push [branch-or-path]`

Push a worktree branch, optionally create a PR, and optionally remove the worktree afterward.

```bash
bonsai push
bonsai push feat/search
bonsai push --pr
bonsai push --web
bonsai push --pr --remove
```

### `bonsai clean`

Open an interactive picker for merged, stale, or otherwise removable worktrees.

```bash
bonsai clean
bonsai clean --all
bonsai clean --stale 7
bonsai clean --force
```

Keys: `up/down` move, `space` toggle, `a` select all, `n` select none, `enter` confirm, `q` quit

### `bonsai prune`

Review deletion candidates one by one in a non-TUI flow.

```bash
bonsai prune
bonsai prune --dry-run
bonsai prune -y
```

### `bonsai rm <n> [n...]`

Remove worktrees by the numbers shown in `bonsai list`.

```bash
bonsai rm 2
bonsai rm 1 3 5
bonsai rm --dry-run 2
bonsai rm --force 2
```

## More Useful Commands

```bash
bonsai switch      # interactive picker that prints a cd command
bonsai status      # dashboard view of working tree state
bonsai stats       # summary across all worktrees
bonsai sync        # rebase or merge all worktrees from base
bonsai open 2      # open a worktree in your editor
bonsai snapshot    # archive a worktree before deleting it
bonsai doctor      # detect broken or orphaned worktrees
```

## Safety Defaults

- Never deletes worktrees with unpushed commits unless `--force` is used
- Supports `--dry-run` on destructive flows
- Gracefully works without GitHub auth
- Uses the `gh` CLI instead of managing GitHub tokens directly

## Configuration

Global config lives at `~/.config/bonsai/config.toml`.

```toml
stale_threshold_days = 14
default_remote = "origin"
default_base = "main"
ticket_pattern = "([A-Z]+-\\d+)"
```

Per-repo overrides are supported with `.bonsai.toml` at the repo root.

## GitHub Integration

If `gh` is installed and authenticated, bonsai can:

- show PR status in `bonsai list`
- detect merged branches for cleanup
- open PRs from `bonsai push --pr`

Setup:

```bash
gh auth login
```

Without `gh`, bonsai still works. GitHub-specific fields just fall back to unknown status.

## License

MIT
