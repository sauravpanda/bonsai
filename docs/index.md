---
title: Home
nav_order: 1
---

# Bonsai

Safe git worktree cleanup for Claude Code and other coding agents.

Coding agents are very good at creating worktrees and very bad at deleting
them. After a few weeks of agent-assisted work there are dozens scattered
across your machine, quietly holding tens of gigabytes, and no quick way to
tell which are merged, which still hold unpushed commits, and which are in use
right now.

Asking an agent to sort that out means asking a language model to assemble
`git worktree remove --force` and `git branch -D` from whatever it can infer.
One wrong flag deletes work that exists nowhere else.

Bonsai moves that decision out of the model. It finds agent worktrees across
your repositories, classifies each one as `safe`, `review`, or `protected`, and
removes only the `safe` ones — no matter how the agent asks.

## Install

```bash
go install github.com/sauravpanda/bonsai@latest
```

From source:

```bash
git clone https://github.com/sauravpanda/bonsai
cd bonsai
make install
```

Requirements: Go 1.25+, and optionally the
[GitHub CLI](https://cli.github.com/) for PR status and PR creation.

## Two ways in

**Let an agent do it.** Install the Claude Code plugin and ask for a cleanup in
plain language. Claude plans, explains what it will preserve, asks for
approval, and applies only the approved plan. See
[Agent integration]({% link agents.md %}).

**Do it yourself.** Create worktrees, see them in one table, push and open PRs,
and clean up what has merged.

```bash
bonsai new feat/search
bonsai list
bonsai push --pr
bonsai clean --keep-history
bonsai undo
```

See the [command reference]({% link commands.md %}) for every command and flag.

## Where to go next

- [Agent integration]({% link agents.md %}) — the Claude Code plugin, and the
  JSON plan/apply protocol any agent can drive
- [Command reference]({% link commands.md %}) — every command, flag, and
  structured output format
- [Safety model]({% link safety.md %}) — what `safe`, `review`, and `protected`
  mean, and what Bonsai refuses to do
- [Undo and trash]({% link recovery.md %}) — recovering a removed worktree
- [Configuration]({% link configuration.md %}) — global and per-repo settings
