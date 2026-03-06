# gw (Git Worktree manager)

A Git worktree management tool that handles path generation, branch cleanup, and project-specific setup hooks so you don't have to.

## Install

### Homebrew

```bash
brew install satococoa/tap/gw
```

### Go

```bash
go install github.com/satococoa/wtp/v2/cmd/gw@latest
```

### Binary

Download from [GitHub Releases](https://github.com/satococoa/wtp/releases).

### From Source

```bash
git clone https://github.com/satococoa/wtp.git
cd wtp
go build -o gw ./cmd/gw
```

## Usage

```bash
# Create worktree from existing branch (auto-tracks remote if needed)
gw add feature/auth

# Create worktree with a new branch
gw add -b feature/new-feature

# Create new branch from a specific commit
gw add -b hotfix/urgent abc1234

# List all worktrees
gw list

# Remove a worktree
gw remove feature/auth

# Remove worktree and its branch together
gw remove --with-branch feature/auth

# Navigate to a worktree (requires shell integration)
gw cd feature/auth

# Navigate to the main worktree
gw cd @

# Generate a starter .gw.yml
gw init
```

## Configuration

Project hooks are defined in `.gw.yml` at the repository root:

```yaml
version: "1.0"
defaults:
  base_dir: "../worktrees"

hooks:
  post_create:
    - type: copy
      from: ".env"       # relative to main worktree
      to: ".env"         # relative to new worktree

    - type: symlink
      from: ".bin"
      to: ".bin"

    - type: command
      command: "npm ci"
```

Hook types:
- **copy** — copy files/directories from the main worktree (works with gitignored files)
- **symlink** — symlink shared directories from the main worktree
- **command** — run a command in the new worktree (supports `env` and `work_dir`)

## Shell Integration

### Homebrew

Tab completion and `gw cd` work automatically via lazy-loading on first TAB press.

### go install / binary

Add one line to your shell config:

```bash
# Bash (~/.bashrc)
eval "$(gw shell-init bash)"

# Zsh (~/.zshrc)
eval "$(gw shell-init zsh)"

# Fish (~/.config/fish/config.fish)
gw shell-init fish | source
```

This enables both tab completion and the `gw cd` shell hook.

## Worktree Structure

With the default `base_dir: "../worktrees"`, branch names map directly to paths:

```
project/
├── .gw.yml
└── src/

../worktrees/
├── feature/
│   └── auth/        # gw add feature/auth
└── hotfix/
    └── bug-123/     # gw add hotfix/bug-123
```

## Requirements

- Git 2.17+
- Linux (x86_64, ARM64) or macOS (Apple Silicon)
- Bash 4+, Zsh, or Fish (for shell integration)

## Development

```bash
go tool task dev    # fmt, lint, test, build
go tool task test   # unit tests with race detection
go tool task lint   # golangci-lint
go tool task fmt    # gofmt + goimports
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

MIT — see [LICENSE](LICENSE).
