# gw

Git worktree manager. Creates worktrees, runs post-create hooks (copy, symlink, command), and handles shell integration for `gw cd`.

## Install

```bash
go install github.com/jeanduplessis/gw/cmd/gw@latest
```

Or build from source:

```bash
go build -o gw ./cmd/gw
```

## Commands

```
gw add <branch>              # create worktree from existing branch
gw add -b <branch> [commit]  # create worktree with new branch
gw list                      # list all worktrees
gw remove <name>             # remove worktree
gw remove --with-branch <n>  # remove worktree and branch
gw cd <name>                 # navigate to worktree (needs shell hook)
gw cd @                      # navigate to main worktree
gw init                      # generate starter .gw.yml
```

## Config (`.gw.yml`)

```yaml
version: "1.0"
defaults:
  base_dir: "../worktrees"
hooks:
  post_create:
    - type: copy
      from: ".env"
      to: ".env"
    - type: symlink
      from: ".bin"
      to: ".bin"
    - type: command
      command: "npm ci"
```

## Shell Integration

```bash
eval "$(gw shell-init bash)"   # bash
eval "$(gw shell-init zsh)"    # zsh
gw shell-init fish | source    # fish
```

## Development

```bash
go tool task dev    # fmt, lint, test, build
go tool task test   # unit tests
go tool task lint   # golangci-lint
```

## License

MIT
