# bonsai

`bonsai` is a friendly CLI for managing git worktrees.

If AI coding tools keep spawning branches and worktrees all over a repo, bonsai helps keep things tidy. It shows what exists, what is stale, what has a PR, what still has unpushed work, and what is safe to clean up.

For Claude Code, Bonsai also ships a `/bonsai:cleanup` skill. Claude creates a
short-lived cleanup plan, explains what will be preserved, asks for approval,
and applies only the worktrees Bonsai has proven safe.

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

- Go 1.25+
- Optional: [GitHub CLI](https://cli.github.com/) for PR status and PR creation

### Add the Claude Code skill

After installing the `bonsai` binary, run these commands inside Claude Code:

```text
/plugin marketplace add sauravpanda/bonsai
/plugin install bonsai@bonsai-tools
```

Then ask Claude to clean up naturally, or invoke the skill directly:

```text
/bonsai:cleanup
```

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
bonsai push --pr --remove --yes
```

`--remove` asks before deleting the worktree. Add `--yes` only when that
removal has already been approved in an automated workflow.

### `bonsai clean`

Open an interactive picker for merged, stale, or otherwise removable worktrees.
Commands are repository-local by default. Add `--global` to discover Git
repositories under common development roots and manage their linked worktrees
in one view.

```bash
bonsai clean
bonsai clean --all
bonsai clean --global --all
bonsai clean --global --claude
bonsai clean --stale 7
bonsai clean --force
```

Picker keys: `up/down` or `j/k` move, `space` toggles the highlighted row,
`a` selects all safe rows, `n` clears the selection, and `enter` opens a final
review screen. Confirm deletion there with `y` or return with `n`/`esc`.

### `bonsai prune`

Classify merged or inactive worktrees as safe, review, or protected. Automatic
cleanup only removes safe worktrees and their local branches.

```bash
bonsai prune
bonsai prune --dry-run
bonsai prune --claude
bonsai prune --global --claude --dry-run
bonsai prune --global --claude --json
bonsai prune --global --root ~/workspace --dry-run
bonsai prune --apply <plan-id> --yes
bonsai prune -y              # safe worktrees only
```

`--json` saves a plan for 15 minutes. Applying the plan rechecks every local
fingerprint first and aborts before deletion if any worktree changed.

Global scans are bounded to existing common development directories:
`~/Github`, `~/GitHub`, `~/Projects`, `~/Developer`, `~/Code`, and `~/src`.
Use repeatable `--root` flags to choose other locations. Repository-local
`.bonsai.toml` settings are honored independently during a global scan.

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

- Classifies candidates as `safe`, `review`, or `protected`
- Protects staged, modified, untracked, unpushed, locked, current, and open-PR worktrees
- Never lets `--yes` or plan/apply delete review or protected worktrees
- Revalidates saved plans before making any changes
- Deletes local branches only for worktrees proven recoverable; never deletes remote branches
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

Without `gh`, bonsai still works and falls back to local Git safety checks.

## Plain Output

Bonsai automatically disables ANSI colors when stdout is redirected or piped.
Set `NO_COLOR` to any non-empty value or pass the global `--no-color` flag to
force plain output in a terminal:

```bash
NO_COLOR=1 bonsai list
bonsai --no-color status
```

## License

MIT
