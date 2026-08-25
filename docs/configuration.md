---
title: Configuration
nav_order: 6
---

# Configuration

Global config lives at `~/.config/bonsai/config.toml`.

```toml
stale_threshold_days = 14
default_remote = "origin"
default_base = "main"
ticket_pattern = "([A-Z]+-\\d+)"
trash_retention_days = 30
```

Per-repo overrides go in `.bonsai.toml` at the repository root. During a global
scan, each repository's own `.bonsai.toml` is honored independently.

## Managing config

```bash
bonsai config path      # print the config file path
bonsai config init      # write defaults to the standard path
bonsai config show      # show the effective merged configuration
bonsai config check     # validate both layers
bonsai config check --json
```

## Validation

`bonsai config check` validates the global and repository layers together and
prints the merged effective values.

It reports the exact source file and line for invalid TOML or invalid values,
and warns when it finds unknown keys. Syntax, type, and value errors exit
non-zero; unknown-key warnings do not.
