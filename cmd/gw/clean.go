package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/jeanduplessis/gw/internal/command"
	"github.com/jeanduplessis/gw/internal/config"
	"github.com/jeanduplessis/gw/internal/errors"
	"github.com/jeanduplessis/gw/internal/git"
)

// Variable to allow mocking in tests
var cleanGetwd = os.Getwd

// NewCleanCommand creates the clean command definition
func NewCleanCommand() *cli.Command {
	return &cli.Command{
		Name:      "clean",
		Usage:     "Remove worktrees with branches already merged into main",
		UsageText: "gw clean",
		Description: "Scans all managed worktrees and removes those whose branches have\n" +
			"been merged into the main branch. Both the worktree directory and\n" +
			"its local branch are deleted.\n\n" +
			"Examples:\n" +
			"  gw clean   # Remove all merged worktrees",
		Action: cleanCommand,
	}
}

func cleanCommand(_ context.Context, cmd *cli.Command) error {
	w := cmd.Root().Writer
	if w == nil {
		w = os.Stdout
	}

	cwd, err := cleanGetwd()
	if err != nil {
		return errors.DirectoryAccessFailed("access current", ".", err)
	}

	_, err = git.NewRepository(cwd)
	if err != nil {
		return errors.NotInGitRepository()
	}

	executor := command.NewRealExecutor()
	return cleanCommandWithCommandExecutor(w, executor, cwd)
}

// cleanContext holds shared state used when processing each worktree during clean.
type cleanContext struct {
	w                io.Writer
	executor         command.Executor
	cfg              *config.Config
	mainWorktreePath string
	absCwd           string
}

func cleanCommandWithCommandExecutor(
	w io.Writer,
	executor command.Executor,
	cwd string,
) error {
	worktrees, err := listWorktreesForClean(executor)
	if err != nil {
		return err
	}

	mainWorktreePath := findMainWorktreePath(worktrees)

	baseBranch := resolveBaseBranch(executor, worktrees)
	if baseBranch == "" {
		return errors.MainBranchNotFound()
	}

	cfg := loadConfigOrDefault(mainWorktreePath)

	mergedBranches, err := getMergedBranches(executor, baseBranch)
	if err != nil {
		return err
	}

	absCwd, err := filepath.Abs(cwd)
	if err != nil {
		return errors.DirectoryAccessFailed("access current", cwd, err)
	}

	ctx := &cleanContext{
		w:                w,
		executor:         executor,
		cfg:              cfg,
		mainWorktreePath: mainWorktreePath,
		absCwd:           absCwd,
	}

	removedCount := 0
	for _, wt := range worktrees {
		removed, procErr := processWorktreeForClean(ctx, wt, mergedBranches)
		if procErr != nil {
			return procErr
		}
		if removed {
			removedCount++
		}
	}

	if removedCount == 0 {
		if _, err := fmt.Fprintln(w, "No merged worktrees found"); err != nil {
			return err
		}
	}

	return nil
}

func listWorktreesForClean(executor command.Executor) ([]git.Worktree, error) {
	listCmd := command.GitWorktreeList()
	result, err := executor.Execute([]command.Command{listCmd})
	if err != nil {
		return nil, errors.GitCommandFailed("git worktree list", err.Error())
	}
	return parseWorktreesFromOutput(result.Results[0].Output), nil
}

func loadConfigOrDefault(mainWorktreePath string) *config.Config {
	cfg, err := config.LoadConfig(mainWorktreePath)
	if err != nil {
		cfg = &config.Config{
			Defaults: config.Defaults{
				BaseDir: config.DefaultBaseDir,
			},
		}
	}
	return cfg
}

func getMergedBranches(executor command.Executor, mainBranch string) (map[string]bool, error) {
	mergedCmd := command.GitBranchMerged(mainBranch)
	result, err := executor.Execute([]command.Command{mergedCmd})
	if err != nil {
		return nil, errors.GitCommandFailed("git branch --merged", err.Error())
	}
	if len(result.Results) > 0 && result.Results[0].Error != nil {
		return nil, errors.GitCommandFailed("git branch --merged", result.Results[0].Error.Error())
	}
	return parseMergedBranches(result.Results[0].Output), nil
}

// processWorktreeForClean handles a single worktree during the clean operation.
// Returns (true, nil) if the worktree was successfully removed, (false, nil) if skipped,
// or (false, err) if a fatal write error occurred.
func processWorktreeForClean(
	ctx *cleanContext, wt git.Worktree, mergedBranches map[string]bool,
) (bool, error) {
	if shouldSkipWorktree(wt, ctx.cfg, ctx.mainWorktreePath, mergedBranches) {
		return false, nil
	}

	absTargetPath, err := filepath.Abs(wt.Path)
	if err != nil {
		return false, nil
	}

	displayName := getWorktreeNameFromPath(wt.Path, ctx.cfg, ctx.mainWorktreePath, wt.IsMain)

	if isPathWithin(absTargetPath, ctx.absCwd) {
		return false, writeMessage(ctx.w, "Skipping worktree '%s' (currently active)\n", displayName)
	}

	if !removeWorktree(ctx, wt.Path, displayName) {
		return false, nil
	}

	removeBranch(ctx, wt.Branch)

	return true, nil
}

func shouldSkipWorktree(
	wt git.Worktree, cfg *config.Config, mainWorktreePath string, mergedBranches map[string]bool,
) bool {
	if wt.IsMain {
		return true
	}
	if wt.Branch == "" || wt.Branch == detachedKeyword {
		return true
	}
	if !isWorktreeManagedClean(wt.Path, cfg, mainWorktreePath, wt.IsMain) {
		return true
	}
	return !mergedBranches[wt.Branch]
}

// removeWorktree attempts to remove a worktree and prints the result.
// Returns true if removal succeeded, false otherwise.
func removeWorktree(ctx *cleanContext, wtPath, displayName string) bool {
	removeCmd := command.GitWorktreeRemove(wtPath, false)
	result, err := ctx.executor.Execute([]command.Command{removeCmd})
	if err != nil {
		_, _ = fmt.Fprintf(ctx.w, "Failed to remove worktree '%s': %v\n", displayName, err)
		return false
	}
	if len(result.Results) > 0 && result.Results[0].Error != nil {
		detail := formatGitError(&result.Results[0])
		_, _ = fmt.Fprintf(ctx.w, "Failed to remove worktree '%s': %s\n", displayName, detail)
		return false
	}
	_, _ = fmt.Fprintf(ctx.w, "Removed worktree '%s' at %s\n", displayName, wtPath)
	return true
}

// removeBranch attempts to delete a branch and prints the result.
func removeBranch(ctx *cleanContext, branchName string) {
	branchCmd := command.GitBranchDelete(branchName, false)
	result, err := ctx.executor.Execute([]command.Command{branchCmd})
	if err != nil {
		_, _ = fmt.Fprintf(ctx.w, "Warning: failed to remove branch '%s': %v\n", branchName, err)
		return
	}
	if len(result.Results) > 0 && result.Results[0].Error != nil {
		detail := formatGitError(&result.Results[0])
		_, _ = fmt.Fprintf(ctx.w, "Warning: failed to remove branch '%s': %s\n", branchName, detail)
		return
	}
	_, _ = fmt.Fprintf(ctx.w, "Removed branch '%s'\n", branchName)
}

// formatGitError combines git command output with the error for informative messages.
func formatGitError(result *command.Result) string {
	gitOutput := strings.TrimSpace(result.Output)
	if gitOutput != "" {
		return fmt.Sprintf("%v: %s", result.Error, gitOutput)
	}
	return result.Error.Error()
}

func writeMessage(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

// commonDefaultBranches lists well-known default branch names in priority order.
var commonDefaultBranches = []string{"main", "master"}

// resolveBaseBranch determines the canonical base branch for merge comparison.
// It uses a multi-step strategy:
//  1. Query git symbolic-ref refs/remotes/origin/HEAD for the remote default branch.
//  2. Check if any well-known branch name (main, master) exists in the worktree list.
//  3. Fall back to whatever branch the main worktree currently has checked out.
func resolveBaseBranch(executor command.Executor, worktrees []git.Worktree) string {
	// Strategy 1: Ask git for the remote's default branch
	if branch := resolveRemoteDefaultBranch(executor); branch != "" {
		return branch
	}

	// Collect all branch names for lookup
	branchSet := make(map[string]bool, len(worktrees))
	for _, wt := range worktrees {
		if wt.Branch != "" && wt.Branch != detachedKeyword {
			branchSet[wt.Branch] = true
		}
	}

	// Strategy 2: Check for well-known default branch names
	for _, name := range commonDefaultBranches {
		if branchSet[name] {
			return name
		}
	}

	// Strategy 3: Fall back to the main worktree's current branch
	for _, wt := range worktrees {
		if wt.IsMain && wt.Branch != "" && wt.Branch != detachedKeyword {
			return wt.Branch
		}
	}

	return ""
}

// resolveRemoteDefaultBranch queries git for the remote's default branch via symbolic-ref.
// Returns the short branch name (e.g. "main") or "" if unavailable.
func resolveRemoteDefaultBranch(executor command.Executor) string {
	symRefCmd := command.GitSymbolicRef("refs/remotes/origin/HEAD")
	result, err := executor.Execute([]command.Command{symRefCmd})
	if err != nil || len(result.Results) == 0 || result.Results[0].Error != nil {
		return ""
	}
	// Output is like "origin/main" — extract the branch name after the last slash
	output := strings.TrimSpace(result.Results[0].Output)
	if idx := strings.LastIndex(output, "/"); idx >= 0 {
		return output[idx+1:]
	}
	return output
}

// findMainWorktreePath is defined in cd.go and shared across commands.

// parseMergedBranches parses the output of `git branch --merged` into a set of branch names.
func parseMergedBranches(output string) map[string]bool {
	branches := make(map[string]bool)
	for _, line := range strings.Split(output, "\n") {
		// `git branch --merged` output has leading whitespace and optional prefixes:
		//   '* ' for the current branch
		//   '+ ' for branches checked out in other worktrees
		b := strings.TrimSpace(line)
		b = strings.TrimPrefix(b, "* ")
		b = strings.TrimPrefix(b, "+ ")
		if b != "" {
			branches[b] = true
		}
	}
	return branches
}

// isWorktreeManagedClean determines if a worktree is managed by gw (for clean command)
func isWorktreeManagedClean(worktreePath string, cfg *config.Config, mainRepoPath string, isMain bool) bool {
	return isWorktreeManagedCommon(worktreePath, cfg, mainRepoPath, isMain)
}
