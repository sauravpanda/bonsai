# bonsai

Safe git worktree cleanup for Claude Code and other coding agents.

📖 **[Documentation](https://sauravpanda.github.io/bonsai/)**

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

Requires Go 1.25+. The [GitHub CLI](https://cli.github.com/) is optional, and
adds PR status and PR creation.

## Claude Code

```text
/plugin marketplace add sauravpanda/bonsai
/plugin install bonsai@bonsai-tools
```

Then ask Claude to clean up your worktrees, or run `/bonsai:cleanup`. Claude
scans your development directories, explains what it will remove and what it
will preserve, reports the disk space you would reclaim, and asks before
applying anything.

## Other coding agents

Any agent that can run a shell and read JSON can drive the same protocol —
Codex, Cursor, OpenCode, Aider, and custom automation included.

```bash
bonsai prune --global --keep-history --json   # plan
bonsai prune --apply <plan-id> --yes --json   # apply, after approval
bonsai undo --json                            # change your mind
```

Plans expire after 15 minutes and are revalidated before anything is deleted.
See [Agent integration](https://sauravpanda.github.io/bonsai/agents.html) for
the full contract.

## Humans, too

```bash
bonsai new feat/search     # create a worktree and branch
bonsai list                # see everything, with PR status
bonsai push --pr           # push and open a PR
bonsai clean               # interactively remove what has merged
bonsai undo                # restore the last recoverable removal
```

Full [command reference](https://sauravpanda.github.io/bonsai/commands.html).

## Safety

- Classifies every candidate as `safe`, `review`, or `protected`
- Protects dirty, untracked, unpushed, locked, current, and open-PR worktrees
- Never lets `--yes` or plan/apply remove a `review` or `protected` worktree
- Revalidates saved plans before making any changes
- Deletes local branches only when proven recoverable; never remote branches
- Opt-in recovery with `--keep-history`, `bonsai undo`, and `bonsai trash`

More on the [safety model](https://sauravpanda.github.io/bonsai/safety.html).

## Contributing

```bash
git clone https://github.com/sauravpanda/bonsai
cd bonsai
make install
go test ./...
```

## License

MIT
