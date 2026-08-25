---
title: Safety model
nav_order: 4
---

# Safety model

Bonsai's whole design goal is that no agent, and no combination of flags, can
talk it into deleting work that exists nowhere else.

## Three classifications

Every candidate worktree lands in exactly one bucket.

| Class | Meaning |
|---|---|
| `safe` | Bonsai proved the work is recoverable — merged or pushed, clean, unlocked, not in use |
| `review` | Might be removable, but a human should look first |
| `protected` | Bonsai will not remove this, and no flag makes it automatic |

A worktree is `protected` when it has staged changes, modified files,
untracked files, unpushed commits, an open PR, a lock, or is the worktree you
are currently in. A failed inspection or an unknown PR state is also
protected — when Bonsai cannot prove something, it keeps the worktree.

## What Bonsai will not do

- Automatic paths never remove `review` or `protected` worktrees. Neither
  `--yes` nor plan/apply promotes them.
- Saved plans are revalidated before anything is deleted. If a worktree changed
  after planning, apply aborts.
- Plans expire after 15 minutes.
- Local branches are deleted only for worktrees proven recoverable.
- Remote branches are never deleted.
- Destructive flows support `--dry-run`.

`--force` exists for interactive use only. It lets *you* select a `review` or
`protected` worktree in the `clean` picker; it does not change what automatic
cleanup will do, and agents should never use it.

## Recovery

`--keep-history` on `clean`, `prune`, `rm`, or `push --remove` makes a removal
recoverable with `bonsai undo`. See [Undo and trash]({% link recovery.md %}).

## GitHub integration

When the [GitHub CLI](https://cli.github.com/) is installed and authenticated,
Bonsai can show PR status in `bonsai list`, detect merged branches for cleanup,
and open PRs from `bonsai push --pr`.

```bash
gh auth login
```

Without `gh`, Bonsai still works and falls back to local Git safety checks.
Because an unknown PR state is treated as protected, losing GitHub access makes
Bonsai more conservative, never less.
