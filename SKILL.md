# gw — Git Worktree Manager

`gw` manages git worktrees with automatic path generation, branch tracking, post-create hooks, and shell navigation.

## Quick Reference

```bash
gw add <branch>                    # worktree from existing/remote branch
gw add -b <new-branch> [commit]    # worktree with new branch
gw list                            # list worktrees (aliases: ls)
gw list -q                         # paths only
gw remove <name>                   # remove worktree (aliases: rm)
gw remove --with-branch <name>     # remove worktree + branch
gw remove -f <name>                # force remove dirty worktree
gw cd <name>                       # print worktree path (shell hook does cd)
gw cd @                            # main worktree
gw cd                              # main worktree (default)
gw init                            # create .gw.yml with examples
```

## Worktree Naming

- `@` always refers to the main worktree.
- Other worktrees are identified by their path relative to `base_dir` (e.g. `feature/auth`, `hotfix/bug-123`).
- Branch names map directly to directory structure under `base_dir`.

## Configuration

File: `.gw.yml` in the repository root.

```yaml
version: "1.0"
defaults:
  base_dir: "../worktrees"    # relative to repo root
hooks:
  post_create:
    - type: copy              # copy from main worktree
      from: ".env"
      to: ".env"
    - type: symlink           # symlink from main worktree
      from: ".bin"
      to: ".bin"
    - type: command           # run in new worktree
      command: "npm ci"
      # env:                  # optional env vars
      # work_dir:             # optional working directory
```

Hook types: `copy`, `symlink`, `command`. Paths in `copy`/`symlink` are relative to the main worktree (`from`) and new worktree (`to`).

## Shell Integration

Required for `gw cd` to actually change directories (without it, `gw cd` only prints the path).

```bash
eval "$(gw shell-init bash)"    # bash — completion + cd hook
eval "$(gw shell-init zsh)"     # zsh
gw shell-init fish | source     # fish
```

## Typical Workflow

```bash
gw add feature/auth             # creates ../worktrees/feature/auth
gw cd feature/auth              # cd into it (with shell hook)
# ... work ...
gw cd @                         # back to main worktree
gw remove --with-branch feature/auth  # cleanup
```

## Error Patterns

- No `.gw.yml`: `gw add` fails with config load error. Run `gw init` first or create manually.
- Branch already checked out: use a different branch or remove the existing worktree.
- Cannot remove current worktree: `cd` out of it first.
- `--force-branch` requires `--with-branch`.
