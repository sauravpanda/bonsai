---
name: cleanup
description: Safely analyze and clean up Claude Code git worktrees with Bonsai. Use when the user asks to clean, prune, remove, audit, or reclaim disk space from Claude Code worktrees, stale agent worktrees, or .claude/worktrees directories.
---

# Clean up Claude Code worktrees

Use Bonsai's saved plan/apply protocol. Keep deletion decisions in the binary
so the workflow remains deterministic and resistant to stale state.

## Workflow

1. Verify that `bonsai` is available. If it is missing, stop and give the user
   the installation command from the project README. Do not install it without
   permission.
2. Run `bonsai prune --global --claude --json`. Bonsai searches bounded
   development roots such as `~/Github`, `~/Projects`, and `~/Developer`. If
   the user names a different location, add a repeatable `--root <directory>`.
3. Read the JSON and summarize:
   - number of worktrees scanned;
   - every `safe` worktree and why it is safe;
   - every `review` and `protected` worktree and why it will be preserved;
   - `reclaimable_bytes` in a human-readable unit.
4. If `safe` is empty, report that nothing is safe to prune and stop.
5. Ask the user to approve applying the displayed plan. Mention the exact
   number of safe worktrees and reclaimable space.
6. After approval, apply that exact plan with:

   ```bash
   bonsai prune --apply <plan_id> --yes --json
   ```

7. Report `removed`, `branches_deleted`, and `reclaimed_bytes`. Call out any
   branch deletion failures.

## Safety rules

- Never use `--force`, `git worktree remove`, or `git branch -D` directly.
- Keep `--global --claude` on the planning command unless the user explicitly
  limits cleanup to the current repository.
- Never apply a different plan ID from the one the user approved.
- Never delete `review` or `protected` entries.
- If applying reports that the plan expired or a worktree changed, create a new
  plan, show the differences, and ask for approval again.
- Keep JSON stdout intact for parsing. Do not merge progress or explanatory
  text into it.
- Trust Bonsai's classification. A failed local inspection or an explicit
  unknown PR state is protected; do not work around it. If GitHub is entirely
  unavailable, Bonsai may still prove recovery from local Git metadata.
