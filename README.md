# bonsai

A CLI tool for managing git worktrees. `git worktree` is barebones — as AI-assisted workflows (Claude Code, Cursor, etc.) spawn worktrees per task, they accumulate fast with no good way to audit, clean, or finalize them.

Bonsai fixes that.

> Created by [Saurav Panda](https://github.com/sauravpanda)

---

## Install

```bash
# From source
git clone https://github.com/sauravpanda/bonsai
cd bonsai
make install          # installs to /usr/local/bin/bonsai
```

Or with `go install`:

```bash
go install github.com/sauravpanda/bonsai@latest
```

**Requirements:** Go 1.21+. GitHub integration requires the [gh CLI](https://cli.github.com/) to be installed and authenticated (`gh auth login`). Bonsai works without it — PR status will just show as unknown.

---

## Commands

### `bonsai list`

Table view of all worktrees with path, branch, age, last commit, ahead/behind base, and PR status. Columns scale to your terminal width.

```
  #   PATH                              BRANCH                AGE    LAST COMMIT              +/-      PR
  ──────────────────────────────────────────────────────────────────────────────────────────────────────
      ~/projects/myapp                  main                  2h     chore: bump deps         +0/-0    —
  1   .claude/worktrees/feat-auth       feat/auth             3d     add OAuth flow           +4/-0    open
  2   .claude/worktrees/fix-payments    fix/payments          21d    fix stripe webhook       +0/-0    merged
  3   .claude/worktrees/feat-dashboard  feat/dashboard        8d     WIP: new dashboard       +2/-0    none
  ──────────────────────────────────────────────────────────────────────────────────────────────────────
  4 worktree(s)  ·  3 added  ·  * unpushed commits
```

| Flag | Description |
|---|---|
| `--no-pr` | Filter: show only worktrees with no PR |
| `--offline` | Skip GitHub PR lookup (faster) |

---

### `bonsai rm <n> [n...]`

Delete worktrees by the numbers shown in `bonsai list`.

```bash
bonsai rm 2            # delete worktree #2
bonsai rm 1 3 5        # delete multiple at once
bonsai rm --force 2    # delete even if branch has unpushed commits
bonsai rm --dry-run 2  # preview without deleting
```

Refuses to delete worktrees with unpushed commits unless `--force` is passed.

---

### `bonsai clean`

Interactive TUI picker. Shows deletion candidates (merged PR, stale, no unpushed commits) and lets you toggle which ones to remove.

```bash
bonsai clean               # candidates only (merged/stale/no unpushed)
bonsai clean --all         # show all worktrees
bonsai clean --stale 7     # override stale threshold to 7 days
bonsai clean --force       # allow dirty removes
```

Keys: `↑/↓` navigate · `space` toggle · `a` select all · `n` select none · `enter` confirm · `q` quit

---

### `bonsai prune`

Non-interactive version of clean. Shows each candidate with its reason and asks for confirmation before deleting.

```bash
bonsai prune               # interactive per-item confirmation
bonsai prune --dry-run     # list candidates, no deletions
bonsai prune -y            # auto-confirm all
bonsai prune --stale 7     # override stale threshold
```

---

### `bonsai push [branch-or-path]`

Push a worktree's branch to the remote. If no argument is given, uses the current working directory.

```bash
bonsai push                      # push current worktree's branch
bonsai push feat/auth            # push by branch name
bonsai push --pr                 # push + open PR via gh
bonsai push --web                # push + open PR creation in browser
bonsai push --remove             # push then remove the worktree
bonsai push --pr --remove        # push, open PR, then clean up
bonsai push --dry-run            # preview only
```

---

## Configuration

Bonsai reads `~/.config/bonsai/config.toml` on startup. Missing keys fall back to defaults.

```toml
stale_threshold_days = 14   # worktrees older than this are prune candidates
default_remote       = "origin"
default_base         = "main"
```

---

## Design decisions

- **Never deletes worktrees with unpushed commits** without explicit `--force`
- **PR merge detection** is the killer feature — if the remote branch is merged, zero-friction delete
- **`--dry-run` on all destructive commands**
- **Delegates to `gh` CLI** for GitHub auth and PR status — no token management
- **Gracefully degrades** when `gh` is not installed or not authenticated
- **Single binary**, no runtime dependencies

---

## GitHub integration

Bonsai uses the `gh` CLI to check PR status and open PRs. It never manages GitHub tokens directly.

```bash
gh auth login    # one-time setup
bonsai list      # PR column now shows open/merged/closed/none
```

Without `gh` auth, the PR column shows `?` and prune/clean candidates are based on staleness only.

---

## License

MIT
