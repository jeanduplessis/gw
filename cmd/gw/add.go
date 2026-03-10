// Package main provides the entrypoint for the gw CLI commands.
package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jeanduplessis/gw/internal/command"
	"github.com/jeanduplessis/gw/internal/config"
	"github.com/jeanduplessis/gw/internal/errors"
	"github.com/jeanduplessis/gw/internal/git"
	"github.com/jeanduplessis/gw/internal/hooks"
	wtpio "github.com/jeanduplessis/gw/internal/io"
)

// NewAddCommand creates the add command definition
func NewAddCommand() *cli.Command {
	return &cli.Command{
		Name:      "add",
		Usage:     "Create a new worktree",
		UsageText: "gw add <branch>\n       gw add -b <new-branch> [<start-point>]",
		Description: "Creates a new worktree for the specified branch. If the branch exists locally " +
			"or on a remote, it will be checked out. If the branch doesn't exist anywhere, " +
			"it will be created automatically.\n\n" +
			"Examples:\n" +
			"  gw add feature/auth                    # Existing branch or creates new one\n" +
			"  gw add -b hotfix/urgent main           # Create new branch from specific start point",
		ShellComplete: completeBranches,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "branch",
				Usage:   "Create new branch",
				Aliases: []string{"b"},
			},
		},
		Action: addCommand,
	}
}

func addCommand(_ context.Context, cmd *cli.Command) error {
	// Get the writer from cli.Command
	w := cmd.Root().Writer
	if w == nil {
		w = os.Stdout
	}
	// Wrap in FlushingWriter to ensure real-time output for all operations
	fw := wtpio.NewFlushingWriter(w)
	// Validate inputs
	if err := validateAddInput(cmd); err != nil {
		return err
	}

	// Setup repository and configuration
	_, cfg, mainRepoPath, err := setupRepoAndConfig()
	if err != nil {
		return err
	}

	// Create command executor
	executor := command.NewRealExecutor()

	return addCommandWithCommandExecutor(cmd, fw, executor, cfg, mainRepoPath)
}

// addCommandWithCommandExecutor is the new implementation using CommandExecutor
func addCommandWithCommandExecutor(
	cmd *cli.Command, w io.Writer, cmdExec command.Executor, cfg *config.Config, mainRepoPath string,
) error {
	// Resolve worktree path and branch name
	var firstArg string
	if cmd.Args().Len() > 0 {
		firstArg = cmd.Args().Get(0)
	}

	workTreePath, branchName := resolveWorktreePath(cfg, mainRepoPath, firstArg, cmd)

	// Resolve branch if needed
	resolvedTrack, branchNotFound, err := resolveBranchTracking(cmd, branchName, mainRepoPath)
	if err != nil {
		return err
	}

	// Build git worktree command using the new command builder
	worktreeCmd := buildWorktreeCommand(cmd, workTreePath, branchName, resolvedTrack, branchNotFound)

	// Execute the command
	result, err := cmdExec.Execute([]command.Command{worktreeCmd})
	if err != nil {
		return err
	}

	// Check if command succeeded
	if len(result.Results) > 0 && result.Results[0].Error != nil {
		gitError := result.Results[0].Error
		gitOutput := result.Results[0].Output

		// Analyze git error output for better error messages
		return analyzeGitWorktreeError(workTreePath, branchName, gitError, gitOutput)
	}

	if err := executePostCreateHooks(w, cfg, mainRepoPath, workTreePath); err != nil {
		if _, warnErr := fmt.Fprintf(w, "Warning: Hook execution failed: %v\n", err); warnErr != nil {
			return warnErr
		}
	}

	if err := displaySuccessMessage(w, branchName, workTreePath, cfg, mainRepoPath); err != nil {
		return err
	}

	return nil
}

// buildWorktreeCommand builds a git worktree command using the new command package.
// When autoCreate is true, the branch doesn't exist and will be created with -b.
func buildWorktreeCommand(
	cmd *cli.Command, workTreePath, branchName, resolvedTrack string, autoCreate bool,
) command.Command {
	opts := command.GitWorktreeAddOptions{
		Branch: cmd.String("branch"),
	}

	// Auto-create: branch not found locally or remotely, so create it
	if autoCreate && opts.Branch == "" {
		opts.Branch = branchName
	}

	// Use resolved track if provided
	if resolvedTrack != "" {
		opts.Track = resolvedTrack
	}

	var commitish string

	// Handle different argument patterns based on flags
	if resolvedTrack != "" {
		// When using resolved tracking, the commitish is the remote branch
		commitish = resolvedTrack
		// If there's an argument, it's the local branch name (not used as commitish)
		if cmd.Args().Len() > 0 && opts.Branch == "" {
			// The first argument is the branch name when using resolved tracking without -b
			opts.Branch = cmd.Args().Get(0)
		}
	} else if autoCreate {
		// Auto-create: no commitish needed, git will branch from HEAD
	} else if cmd.Args().Len() > 0 {
		// Normal case: first argument is the branch/commitish
		commitish = cmd.Args().Get(0)
		// If branch creation with -b, second arg (if any) is the commitish
		if opts.Branch != "" && cmd.Args().Len() > 1 {
			commitish = cmd.Args().Get(1)
		}
	}

	return command.GitWorktreeAdd(workTreePath, commitish, opts)
}

// analyzeGitWorktreeError analyzes git worktree errors and provides specific error messages
func analyzeGitWorktreeError(workTreePath, branchName string, gitError error, gitOutput string) error {
	errorOutput := strings.ToLower(gitOutput)

	// Check for specific error types
	if isBranchNotFoundError(errorOutput) {
		return errors.BranchNotFound(branchName)
	}

	if isWorktreeAlreadyExistsError(errorOutput) {
		return &WorktreeAlreadyExistsError{
			BranchName: branchName,
			Path:       workTreePath,
			GitError:   gitError,
		}
	}

	if isBranchAlreadyExistsError(errorOutput) {
		return &BranchAlreadyExistsError{
			BranchName: branchName,
			GitError:   gitError,
		}
	}

	if isPathAlreadyExistsError(errorOutput) {
		return &PathAlreadyExistsError{
			Path:     workTreePath,
			GitError: gitError,
		}
	}

	if isMultipleBranchesError(errorOutput) {
		return &MultipleBranchesError{
			BranchName: branchName,
			GitError:   gitError,
		}
	}

	if isInvalidPathError(errorOutput, workTreePath, gitOutput) {
		return fmt.Errorf(`failed to create worktree at '%s'

The git command failed to create the worktree directory.

Possible causes:
  • Invalid path specified
  • Parent directory doesn't exist
  • Insufficient permissions
  • Path points to a file instead of directory

Details: %s

Tip: Check that the parent directory exists and you have write permissions.

Original error: %w`, workTreePath, gitOutput, gitError)
	}

	// Default error with helpful context
	return fmt.Errorf(`worktree creation failed for path '%s'

The git command encountered an error while creating the worktree.

Details: %s

Tip: Run 'git worktree list' to see existing worktrees, or check git documentation for valid worktree paths.

Original error: %w`, workTreePath, gitOutput, gitError)
}

// Helper functions to reduce cyclomatic complexity
func isBranchNotFoundError(errorOutput string) bool {
	return strings.Contains(errorOutput, "invalid reference") ||
		strings.Contains(errorOutput, "not a valid object name") ||
		(strings.Contains(errorOutput, "pathspec") && strings.Contains(errorOutput, "did not match"))
}

func isWorktreeAlreadyExistsError(errorOutput string) bool {
	return strings.Contains(errorOutput, "already checked out") ||
		strings.Contains(errorOutput, "already used by worktree")
}

func isBranchAlreadyExistsError(errorOutput string) bool {
	return strings.Contains(errorOutput, "branch") &&
		strings.Contains(errorOutput, "already exists")
}

func isPathAlreadyExistsError(errorOutput string) bool {
	return strings.Contains(errorOutput, "already exists")
}

func isMultipleBranchesError(errorOutput string) bool {
	return strings.Contains(errorOutput, "more than one remote") || strings.Contains(errorOutput, "ambiguous")
}

func isInvalidPathError(errorOutput, workTreePath, gitOutput string) bool {
	return strings.Contains(errorOutput, "could not create directory") ||
		strings.Contains(errorOutput, "unable to create") ||
		strings.Contains(errorOutput, "is not a directory") ||
		strings.Contains(errorOutput, "fatal:") ||
		strings.Contains(workTreePath, "/dev/") ||
		gitOutput == ""
}

// WorktreeAlreadyExistsError reports that a branch already has an attached worktree.
type WorktreeAlreadyExistsError struct {
	BranchName string
	Path       string
	GitError   error
}

func (e *WorktreeAlreadyExistsError) Error() string {
	return fmt.Sprintf(`worktree for branch '%s' already exists

The branch '%s' is already checked out in another worktree.

Solutions:
  • Use '--force' flag to allow multiple checkouts
  • Choose a different branch
  • Remove the existing worktree first

Original error: %v`, e.BranchName, e.BranchName, e.GitError)
}

// BranchAlreadyExistsError indicates that a branch creation request conflicts with an existing branch.
type BranchAlreadyExistsError struct {
	BranchName string
	GitError   error
}

func (e *BranchAlreadyExistsError) Error() string {
	return fmt.Sprintf(`branch '%s' already exists

The branch '%s' already exists in this repository.

Solutions:
  • Run 'gw add %s' to create a worktree for the existing branch
  • Choose a different branch name with '--branch'
  • Delete the existing branch if it's no longer needed

Original error: %v`, e.BranchName, e.BranchName, e.BranchName, e.GitError)
}

// PathAlreadyExistsError indicates that the destination directory already exists.
type PathAlreadyExistsError struct {
	Path     string
	GitError error
}

func (e *PathAlreadyExistsError) Error() string {
	return fmt.Sprintf(`destination path already exists: %s

The target directory already exists and is not empty.

Solutions:
  • Use --force flag to overwrite existing directory
  • Remove the existing directory
  • Use a different branch name

Original error: %v`, e.Path, e.GitError)
}

// MultipleBranchesError reports that a branch name resolves to multiple remotes and needs disambiguation.
type MultipleBranchesError struct {
	BranchName string
	GitError   error
}

func (e *MultipleBranchesError) Error() string {
	return fmt.Sprintf(`branch '%s' exists in multiple remotes

Use the --track flag to specify which remote to use:
  • gw add --track origin/%s %s
  • gw add --track upstream/%s %s

Original error: %v`, e.BranchName, e.BranchName, e.BranchName, e.BranchName, e.BranchName, e.GitError)
}

func executePostCreateHooks(w io.Writer, cfg *config.Config, repoPath, workTreePath string) error {
	if cfg.HasHooks() {
		if _, err := fmt.Fprintln(w, "\nExecuting post-create hooks..."); err != nil {
			return err
		}

		executor := hooks.NewExecutor(cfg, repoPath)
		if err := executor.ExecutePostCreateHooks(w, workTreePath); err != nil {
			return err
		}

		if _, err := fmt.Fprintln(w, "✓ All hooks executed successfully"); err != nil {
			return err
		}
	}
	return nil
}

func validateAddInput(cmd *cli.Command) error {
	if cmd.Args().Len() == 0 && cmd.String("branch") == "" {
		return errors.BranchNameRequired("gw add <branch> | -b <new-branch> [<start-point>]")
	}

	return nil
}

func setupRepoAndConfig() (*git.Repository, *config.Config, string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, "", errors.DirectoryAccessFailed("access current", ".", err)
	}

	repo, err := git.NewRepository(cwd)
	if err != nil {
		return nil, nil, "", errors.NotInGitRepository()
	}

	mainRepoPath, err := repo.GetMainWorktreePath()
	if err != nil {
		mainRepoPath = repo.Path()
	}

	cfg, err := config.LoadConfig(mainRepoPath)
	if err != nil {
		configPath := mainRepoPath + "/.gw.yml"
		return nil, nil, "", errors.ConfigLoadFailed(configPath, err)
	}

	return repo, cfg, mainRepoPath, nil
}

// getBranches gets available branch names and writes them to the writer (testable)
func getBranches(w io.Writer) error {
	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	// Get all branches using git for-each-ref for better control
	gitCmd := exec.Command("git", "for-each-ref", "--format=%(refname:short)", "refs/heads", "refs/remotes")
	gitCmd.Dir = cwd
	output, err := gitCmd.Output()
	if err != nil {
		return err
	}

	branches := strings.Split(strings.TrimSpace(string(output)), "\n")

	// Use a map to avoid duplicates
	seen := make(map[string]bool)

	for _, branch := range branches {
		if branch == "" {
			continue
		}

		// Skip HEAD references and bare origin
		if branch == "origin/HEAD" || branch == "origin" {
			continue
		}

		// Remove remote prefix for display, but keep track of what we've seen
		displayName := branch
		if strings.HasPrefix(branch, "origin/") {
			// For remote branches, show without the origin/ prefix
			displayName = strings.TrimPrefix(branch, "origin/")
		}

		// Skip if already seen (handles case where local and remote have same name)
		if seen[displayName] {
			continue
		}

		seen[displayName] = true
		if _, err := fmt.Fprintln(w, displayName); err != nil {
			return err
		}
	}

	return nil
}

// completeBranches provides branch name completion for urfave/cli (wrapper for getBranches)
func completeBranches(_ context.Context, cmd *cli.Command) {
	current, previous := completionArgsFromCommand(cmd)
	if maybeCompleteFlagSuggestions(cmd, current, previous) {
		return
	}

	var buf bytes.Buffer
	if err := getBranches(&buf); err != nil {
		return
	}

	// Output each line using fmt.Println for urfave/cli compatibility
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		if _, err := fmt.Println(scanner.Text()); err != nil {
			return
		}
	}
}

// displaySuccessMessage is a convenience wrapper for displaySuccessMessageWithCommitish
func displaySuccessMessage(
	w io.Writer,
	branchName, workTreePath string,
	cfg *config.Config,
	mainRepoPath string,
) error {
	return displaySuccessMessageWithCommitish(w, branchName, workTreePath, "", cfg, mainRepoPath)
}

func displaySuccessMessageWithCommitish(
	w io.Writer, branchName, workTreePath, commitish string, cfg *config.Config, mainRepoPath string,
) error {
	if _, err := fmt.Fprintln(w, "✅ Worktree created successfully!"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "📁 Location: %s\n", workTreePath); err != nil {
		return err
	}

	if branchName != "" {
		if _, err := fmt.Fprintf(w, "🌿 Branch: %s\n", branchName); err != nil {
			return err
		}
	} else if commitish != "" {
		if _, err := fmt.Fprintf(w, "🏷️  Commit: %s\n", commitish); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "💡 To switch to the new worktree, run:"); err != nil {
		return err
	}

	// Use the consistent worktree naming logic
	isMain := isMainWorktree(workTreePath, mainRepoPath)
	worktreeName := getWorktreeNameFromPath(workTreePath, cfg, mainRepoPath, isMain)
	if _, err := fmt.Fprintf(w, "   gw cd %s\n", worktreeName); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "   If gw cd only prints a path, set up the shell hook first:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "   eval \"$(gw shell-init %s)\"\n", detectShellOrDefault()); err != nil {
		return err
	}

	return nil
}

// detectShellOrDefault returns the user's shell name, falling back to "<shell>" as a placeholder.
func detectShellOrDefault() string {
	shell := getShellFunc()
	if shell == "" {
		return "<shell>"
	}
	return shell
}

// isMainWorktree checks if the given path is the main worktree
func isMainWorktree(workTreePath, mainRepoPath string) bool {
	absWorkTreePath, err := filepath.Abs(workTreePath)
	if err != nil {
		return false
	}

	absMainRepoPath, err := filepath.Abs(mainRepoPath)
	if err != nil {
		return false
	}

	return absWorkTreePath == absMainRepoPath
}

// resolveWorktreePath determines the worktree path and branch name based on arguments
func resolveWorktreePath(
	cfg *config.Config, repoPath, firstArg string, cmd *cli.Command,
) (workTreePath, branchName string) {
	// Generate path automatically from branch name
	branchName = firstArg

	// If -b flag is provided, use that as the branch name for path generation
	if newBranch := cmd.String("branch"); newBranch != "" {
		branchName = newBranch
	}

	// If still no branch name, try to use the first argument
	if branchName == "" && firstArg != "" {
		branchName = firstArg
	}

	workTreePath = cfg.ResolveWorktreePath(repoPath, branchName)
	return workTreePath, branchName
}

// resolveBranchTracking handles branch resolution and tracking setup.
// When branchNotFound is true, the caller should auto-create the branch.
func resolveBranchTracking(
	cmd *cli.Command, branchName string, mainRepoPath string,
) (trackRef string, branchNotFound bool, err error) {
	// Only auto-resolve branch when not creating a new branch and branch name exists
	if cmd.String("branch") != "" || branchName == "" {
		return "", false, nil
	}

	repo, err := git.NewRepository(mainRepoPath)
	if err != nil {
		return "", false, err
	}

	// Check if branch exists locally or in remotes
	resolvedBranch, isRemote, err := repo.ResolveBranch(branchName)
	if err != nil {
		// Check if it's a multiple branches error
		if strings.Contains(err.Error(), "exists in multiple remotes") {
			return "", false, &MultipleBranchesError{
				BranchName: branchName,
				GitError:   err,
			}
		}
		// Branch not found locally or remotely — signal auto-create
		if strings.Contains(err.Error(), "not found in local or remote") {
			return "", true, nil
		}
		return "", false, err
	}

	// If it's a remote branch, we need to set up tracking
	if isRemote {
		return resolvedBranch, false, nil
	}

	return "", false, nil
}
