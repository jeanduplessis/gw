package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jeanduplessis/gw/internal/config"
	"github.com/jeanduplessis/gw/internal/errors"
	"github.com/jeanduplessis/gw/internal/git"
)

const configFileMode = 0o600

// Variable to allow mocking in tests
var osGetwd = os.Getwd
var writeFile = os.WriteFile

// NewInitCommand creates the init command definition
func NewInitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize configuration file",
		Description: "Creates a .gw.yml configuration file in the repository root " +
			"with example hooks and settings.",
		Action: initCommand,
	}
}

func initCommand(_ context.Context, cmd *cli.Command) error {
	// Get current working directory (should be a git repository)
	cwd, err := osGetwd()
	if err != nil {
		return errors.DirectoryAccessFailed("access current", ".", err)
	}

	// Initialize repository
	repo, err := git.NewRepository(cwd)
	if err != nil {
		return errors.NotInGitRepository()
	}

	// Get the writer from cli.Command
	w := cmd.Root().Writer
	if w == nil {
		w = os.Stdout
	}

	r := cmd.Root().Reader
	if r == nil {
		r = os.Stdin
	}

	// Check if config file already exists
	configPath := fmt.Sprintf("%s/%s", repo.Path(), config.ConfigFileName)
	if _, statErr := os.Stat(configPath); statErr == nil {
		// Config exists — ask user whether to overwrite or skip
		if !promptConfigOverwrite(w, r, configPath) {
			if _, printErr := fmt.Fprintln(w,
				"Skipping config creation — existing .gw.yml preserved."); printErr != nil {
				return printErr
			}
			// Proceed to shell setup
			_, _ = promptShellSetup(w, r)
			return nil
		}
	}

	repoInfo, repoStatErr := os.Stat(repo.Path())
	if repoStatErr != nil {
		return errors.DirectoryAccessFailed("access repository", repo.Path(), repoStatErr)
	}

	if repoInfo.Mode().Perm()&0o222 == 0 {
		return errors.DirectoryAccessFailed(
			"create configuration file",
			configPath,
			fmt.Errorf("repository directory is read-only"),
		)
	}

	// Create configuration with comments
	configContent := `# gw Configuration
version: "1.0"

# Default settings for worktrees
defaults:
  # Base directory for worktrees (relative to repository root)
  base_dir: ../worktrees

# Hooks that run after creating a worktree
hooks:
  post_create:
    # Example: Copy gitignored files from MAIN worktree to new worktree
    # Note: 'from' is relative to main worktree, 'to' is relative to new worktree
    # - type: copy
    #   from: .env        # Copy actual .env file (gitignored)
    #   to: .env

    # Example: Run a command to show all worktrees
    - type: command
      command: gw list

    # More examples (commented out):
    
    # Copy AI context files (typically gitignored):
    # - type: copy
    #   from: .claude     # Claude AI context
    #   to: .claude
    # - type: copy
    #   from: .cursor/    # Cursor IDE settings
    #   to: .cursor/

    # Share directories with symlinks:
    # - type: symlink
    #   from: .bin        # Shared tool cache
    #   to: .bin
    
    # Run setup commands:
    # - type: command
    #   command: npm install
    # - type: command
    #   command: echo "Created new worktree!"
`

	if err := ensureWritableDirectory(repo.Path()); err != nil {
		return errors.DirectoryAccessFailed("create configuration file", repo.Path(), err)
	}

	// Write configuration file with comments
	if err := writeFile(configPath, []byte(configContent), configFileMode); err != nil {
		return errors.DirectoryAccessFailed("create configuration file", configPath, err)
	}

	if _, printErr := fmt.Fprintf(w, "Configuration file created: %s\n", configPath); printErr != nil {
		return printErr
	}
	if _, printErr := fmt.Fprintln(w, "Edit this file to customize your worktree setup."); printErr != nil {
		return printErr
	}

	// Offer to set up shell integration
	_, _ = promptShellSetup(w, r)

	return nil
}

// promptConfigOverwrite asks the user whether to overwrite an existing config file.
// Returns true if the user wants to overwrite, false otherwise.
// In non-TTY mode, returns false (skip overwrite) without prompting.
func promptConfigOverwrite(w io.Writer, r io.Reader, configPath string) bool {
	if !isTerminalFunc() {
		return false
	}

	prompt := fmt.Sprintf("Overwrite existing %s? [y/N] ", configPath)
	if _, printErr := fmt.Fprint(w, prompt); printErr != nil {
		return false
	}

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false
	}

	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	return answer == "y" || answer == "yes"
}

func ensureWritableDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", filepath.Base(path))
	}

	if info.Mode().Perm()&0o200 == 0 {
		return fmt.Errorf("write permission denied for directory: %s", path)
	}

	return nil
}
