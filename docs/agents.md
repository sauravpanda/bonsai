---
title: Agent integration
nav_order: 2
---

# Agent integration

Bonsai is built so that an agent never has to assemble a destructive Git
command. The agent's job is to run a scan, explain the result, and ask you.
Bonsai's job is to decide what is actually removable.

## Claude Code

Install the binary, then the bundled plugin:

```text
/plugin marketplace add sauravpanda/bonsai
/plugin install bonsai@bonsai-tools
```

Ask Claude to clean up your worktrees in plain language, or invoke the skill
directly:

```text
/bonsai:cleanup
```

Claude scans the usual development directories, summarizes every `safe`,
`review`, and `protected` result with the reason for each, reports how much
disk space the cleanup would reclaim, and asks for approval before applying the
exact plan it showed you. The plugin enables recovery, so the most recent
removal can be restored with `bonsai undo`.

## Any other agent

Any agent that can run a shell and read JSON can drive the same protocol. This
works with Codex, Cursor, OpenCode, Aider, shell-based agents, and custom
automation.

```bash
# 1. Create a recoverable, machine-readable cleanup plan.
bonsai prune --global --keep-history --json

# 2. Let the agent explain the plan and ask you to approve it.

# 3. Apply only that exact plan after approval.
bonsai prune --apply <plan-id> --yes --json

# 4. Restore the newest removal if you change your mind.
bonsai undo --json
```

A good agent instruction is:

> Use Bonsai's JSON plan/apply workflow to find safe worktrees globally. Enable
> keep-history, explain every safe, review, and protected result, ask before
> applying the plan, and never run raw Git deletion commands.

## The plan contract

`--json` writes a saved plan and prints it on stdout. The plan is the unit of
approval: the agent shows you one, and applies that exact one.

- Plans expire **15 minutes** after they are created.
- Applying a plan re-fingerprints every worktree in it first. If anything
  changed since planning — a new commit, a new untracked file — apply aborts
  before deleting.
- `--keep-history` is recorded *in the plan*, so the recovery setting you
  approved is the one that applies.
- Plans only ever contain `safe` entries. There is no flag that promotes a
  `review` or `protected` worktree into an automatic removal.

Useful fields:

| Field | Where | Meaning |
|---|---|---|
| `plan_id` | plan | Pass to `--apply` |
| `reclaimable_bytes` | plan | Disk space the cleanup would free |
| `safe` / `review` / `protected` | plan | Classified worktrees, each with a reason |
| `removed` | apply result | Worktrees actually removed |
| `branches_deleted` | apply result | Local branches deleted |
| `reclaimed_bytes` | apply result | Disk space actually freed |
| `trash_entries` | apply result | Recovery IDs, when `--keep-history` was set |

Keep JSON stdout intact for parsing — Bonsai writes progress and spinner text
to stderr, so stdout stays machine-readable.

## Rules worth giving your agent

- Never use `--force`, `git worktree remove`, or `git branch -D` directly.
- Never apply a different plan ID than the one the user approved.
- Never delete `review` or `protected` entries.
- If apply reports that the plan expired or a worktree changed, create a new
  plan, show the differences, and ask again.
- Trust the classification. A failed inspection or an unknown PR state is
  protected on purpose; do not work around it.
