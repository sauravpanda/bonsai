---
title: Undo and trash
nav_order: 5
---

# Undo and trash

Recovery is opt-in. Add `--keep-history` to `clean`, `prune`, `rm`, or
`push --remove` and Bonsai preserves each approved removal before deleting it.

A trash entry records the branch and its commit, staged and unstaged binary
patches, and non-ignored untracked files. It keeps the deleted branch commit
reachable rather than copying the whole worktree, so entries stay small.

```bash
bonsai undo                 # restore the newest removal
bonsai undo --json

bonsai trash list           # every recoverable removal
bonsai trash list --json
bonsai trash restore <id>   # IDs may be shortened to a unique prefix
bonsai trash empty --yes    # permanently delete all recovery data
```

## What is and is not preserved

Ignored files — dependency directories, build output — are **not** archived.
That is what keeps trash entries small, and it means a restored worktree may
need a fresh `npm install`, `go mod download`, or equivalent.

For a complete copy of a worktree including ignored files, use
[`bonsai snapshot`]({% link commands.md %}#bonsai-snapshot) instead.

## Where entries live

Trash entries are stored under `~/.bonsai/trash` and expire after 30 days by
default. Change that with `trash_retention_days` in
[your config]({% link configuration.md %}).

## Restore refusals

Bonsai refuses to restore over an existing path, or onto a branch that has
moved to a different commit. Both would silently overwrite work, so the restore
stops and tells you instead.
