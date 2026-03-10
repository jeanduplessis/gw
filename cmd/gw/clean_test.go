package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeanduplessis/gw/internal/command"
	"github.com/jeanduplessis/gw/internal/git"
)

// ===== Command Structure Tests =====

func TestNewCleanCommand(t *testing.T) {
	cmd := NewCleanCommand()
	assert.NotNil(t, cmd)
	assert.Equal(t, "clean", cmd.Name)
	assert.Equal(t, "Remove worktrees with branches already merged into main", cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.Action)
}

// ===== Pure Business Logic Tests =====

func TestParseMergedBranches(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		expected map[string]bool
	}{
		{
			name:   "typical merged output",
			output: "  feature/done\n* main\n  fix/typo\n",
			expected: map[string]bool{
				"feature/done": true,
				"main":         true,
				"fix/typo":     true,
			},
		},
		{
			name:     "empty output",
			output:   "",
			expected: map[string]bool{},
		},
		{
			name:   "only main branch",
			output: "* main\n",
			expected: map[string]bool{
				"main": true,
			},
		},
		{
			name:   "multiple branches with whitespace",
			output: "  branch-a\n  branch-b\n  branch-c\n",
			expected: map[string]bool{
				"branch-a": true,
				"branch-b": true,
				"branch-c": true,
			},
		},
		{
			name:   "branches with worktree indicator prefix",
			output: "* main\n+ feature/done\n  feature/other\n",
			expected: map[string]bool{
				"main":          true,
				"feature/done":  true,
				"feature/other": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMergedBranches(tt.output)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveBaseBranch(t *testing.T) {
	tests := []struct {
		name           string
		symRefOutput   string
		symRefError    error
		worktreeList   string
		expectedBranch string
	}{
		{
			name:           "uses remote default branch when available",
			symRefOutput:   "origin/main",
			worktreeList:   "worktree /repo\nHEAD abc123\nbranch refs/heads/develop\n\n",
			expectedBranch: "main",
		},
		{
			name:         "falls back to well-known branch when symbolic-ref fails",
			symRefOutput: "",
			symRefError:  &mockCleanError{message: "not a symbolic ref"},
			worktreeList: "worktree /repo\nHEAD abc123\nbranch refs/heads/develop\n\n" +
				"worktree /worktrees/main-wt\nHEAD def456\nbranch refs/heads/main\n\n",
			expectedBranch: "main",
		},
		{
			name:         "falls back to master when main not present",
			symRefOutput: "",
			symRefError:  &mockCleanError{message: "not a symbolic ref"},
			worktreeList: "worktree /repo\nHEAD abc123\nbranch refs/heads/develop\n\n" +
				"worktree /worktrees/master-wt\nHEAD def456\nbranch refs/heads/master\n\n",
			expectedBranch: "master",
		},
		{
			name:           "falls back to main worktree branch as last resort",
			symRefOutput:   "",
			symRefError:    &mockCleanError{message: "not a symbolic ref"},
			worktreeList:   "worktree /repo\nHEAD abc123\nbranch refs/heads/develop\n\n",
			expectedBranch: "develop",
		},
		{
			name:           "returns empty when no branches available",
			symRefOutput:   "",
			symRefError:    &mockCleanError{message: "not a symbolic ref"},
			worktreeList:   "worktree /repo\nHEAD abc123\ndetached\n\n",
			expectedBranch: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := &mockCleanExecutor{
				results: []command.Result{
					{
						Output: tt.symRefOutput,
						Error:  tt.symRefError,
					},
				},
			}

			worktrees := parseWorktreesFromOutput(tt.worktreeList)
			branch := resolveBaseBranch(mockExec, worktrees)
			assert.Equal(t, tt.expectedBranch, branch)
		})
	}
}

func TestResolveRemoteDefaultBranch(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		err      error
		expected string
	}{
		{
			name:     "parses origin/main",
			output:   "origin/main",
			expected: "main",
		},
		{
			name:     "parses origin/master",
			output:   "origin/master",
			expected: "master",
		},
		{
			name:     "returns empty on error",
			output:   "",
			err:      &mockCleanError{message: "not a symbolic ref"},
			expected: "",
		},
		{
			name:     "returns empty on empty output",
			output:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := &mockCleanExecutor{
				results: []command.Result{
					{Output: tt.output, Error: tt.err},
				},
			}

			result := resolveRemoteDefaultBranch(mockExec)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFindMainWorktreePath_Clean(t *testing.T) {
	tests := []struct {
		name         string
		worktrees    []git.Worktree
		expectedPath string
	}{
		{
			name: "finds main worktree path",
			worktrees: []git.Worktree{
				{Path: "/repo", Branch: "main", IsMain: true},
				{Path: "/worktrees/feature", Branch: "feature"},
			},
			expectedPath: "/repo",
		},
		{
			name:         "no worktrees",
			worktrees:    []git.Worktree{},
			expectedPath: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := findMainWorktreePath(tt.worktrees)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}

// ===== Command Execution Tests =====

func TestCleanCommand_NoMergedWorktrees(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree /repo/../worktrees/feature-wip\nHEAD def456\nbranch refs/heads/feature-wip\n\n",
			},
			{
				// git symbolic-ref (fails - no remote)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n",
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No merged worktrees found")
}

func TestCleanCommand_OneMergedWorktree(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree /repo/../worktrees/feature-done\nHEAD def456\nbranch refs/heads/feature-done\n\n",
			},
			{
				// git symbolic-ref (fails - no remote)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n  feature-done\n",
			},
			{
				// git worktree remove
				Output: "",
			},
			{
				// git branch -d
				Output: "Deleted branch feature-done (was def456).",
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Removed worktree")
	assert.Contains(t, output, "feature-done")
	assert.Contains(t, output, "Removed branch 'feature-done'")

	// Verify correct commands were executed: list, symbolic-ref, merged, remove, branch -d
	assert.Len(t, mockExec.executedCommands, 5)
	assert.Equal(t, []string{"worktree", "list", "--porcelain"}, mockExec.executedCommands[0].Args)
	assert.Equal(t, []string{"symbolic-ref", "--short", "refs/remotes/origin/HEAD"}, mockExec.executedCommands[1].Args)
	assert.Equal(t, []string{"branch", "--merged", "main"}, mockExec.executedCommands[2].Args)
	assert.Equal(t, "remove", mockExec.executedCommands[3].Args[1])
	assert.Equal(t, []string{"branch", "-d", "feature-done"}, mockExec.executedCommands[4].Args)
}

func TestCleanCommand_UsesRemoteDefaultBranch(t *testing.T) {
	// Main worktree is on "develop" but remote default is "main"
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain — main worktree on "develop"
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/develop\n\n" +
					"worktree /repo/../worktrees/feature-done\nHEAD def456\nbranch refs/heads/feature-done\n\n",
			},
			{
				// git symbolic-ref succeeds — remote default is origin/main
				Output: "origin/main",
			},
			{
				// git branch --merged main (uses "main" from remote, not "develop")
				Output: "  feature-done\n",
			},
			{
				// git worktree remove
				Output: "",
			},
			{
				// git branch -d
				Output: "Deleted branch feature-done (was def456).",
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Removed worktree")

	// Verify it used "main" (from remote) not "develop" (from main worktree)
	assert.Equal(t, []string{"branch", "--merged", "main"}, mockExec.executedCommands[2].Args)
}

func TestCleanCommand_MultipleMergedWorktrees(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree /repo/../worktrees/feature-a\nHEAD def456\nbranch refs/heads/feature-a\n\n" +
					"worktree /repo/../worktrees/feature-b\nHEAD ghi789\nbranch refs/heads/feature-b\n\n" +
					"worktree /repo/../worktrees/feature-wip\nHEAD jkl012\nbranch refs/heads/feature-wip\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n  feature-a\n  feature-b\n",
			},
			{Output: ""},               // git worktree remove feature-a
			{Output: "Deleted branch"}, // git branch -d feature-a
			{Output: ""},               // git worktree remove feature-b
			{Output: "Deleted branch"}, // git branch -d feature-b
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "feature-a")
	assert.Contains(t, output, "feature-b")
	assert.NotContains(t, output, "feature-wip")
	assert.NotContains(t, output, "No merged worktrees found")
}

func TestCleanCommand_SkipsCurrentWorktree(t *testing.T) {
	targetPath := "/worktrees/feature-done"
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree " + targetPath + "\nHEAD def456\nbranch refs/heads/feature-done\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n  feature-done\n",
			},
		},
	}

	var buf bytes.Buffer
	// cwd is inside the worktree
	err := cleanCommandWithCommandExecutor(&buf, mockExec, targetPath)

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Skipping worktree")
	assert.Contains(t, output, "currently active")
	// Should still print "no merged worktrees" because none were actually removed
	assert.Contains(t, output, "No merged worktrees found")
}

func TestCleanCommand_SkipsDetachedWorktrees(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree /repo/../worktrees/detached-wt\nHEAD def456\ndetached\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n",
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No merged worktrees found")
	// list + symbolic-ref + merged = 3 commands
	assert.Len(t, mockExec.executedCommands, 3)
}

func TestCleanCommand_SkipsUnmanagedWorktrees(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain: one managed, one unmanaged (outside base_dir)
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree /other/place/feature-done\nHEAD def456\nbranch refs/heads/feature-done\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n  feature-done\n",
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No merged worktrees found")
}

func TestCleanCommand_WorktreeRemovalFails(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree /repo/../worktrees/feature-done\nHEAD def456\nbranch refs/heads/feature-done\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n  feature-done\n",
			},
			{
				// git worktree remove fails
				Output: "fatal: contains modified or untracked files",
				Error:  &mockCleanError{message: "exit status 1"},
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Failed to remove worktree")
	assert.Contains(t, output, "No merged worktrees found")
}

func TestCleanCommand_BranchDeletionFails(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree /repo/../worktrees/feature-done\nHEAD def456\nbranch refs/heads/feature-done\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n  feature-done\n",
			},
			{
				// git worktree remove succeeds
				Output: "",
			},
			{
				// git branch -d fails
				Output: "error: branch not found",
				Error:  &mockCleanError{message: "exit status 1"},
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Removed worktree")
	assert.Contains(t, output, "Warning: failed to remove branch")
	// Should still count as removed since the worktree was removed
	assert.NotContains(t, output, "No merged worktrees found")
}

func TestCleanCommand_OnlyMainWorktree(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain - only main
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n",
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No merged worktrees found")
}

func TestCleanCommand_ListCommandFails(t *testing.T) {
	mockExec := &mockCleanExecutor{
		failOnCall: 0, // Fail on first call (worktree list)
		failError:  "git command failed",
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git worktree list")
}

func TestCleanCommand_MergedCommandFails(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// git worktree list --porcelain
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
		},
		failOnCall: 2, // Fail on third call (branch --merged)
		failError:  "git command failed",
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git branch --merged")
}

func TestCleanCommand_SkipsWorktreesWithEmptyBranch(t *testing.T) {
	mockExec := &mockCleanExecutor{
		results: []command.Result{
			{
				// Worktree with no branch info
				Output: "worktree /repo\nHEAD abc123\nbranch refs/heads/main\n\n" +
					"worktree /repo/../worktrees/no-branch\nHEAD def456\n\n",
			},
			{
				// git symbolic-ref (fails)
				Output: "",
				Error:  &mockCleanError{message: "not a symbolic ref"},
			},
			{
				// git branch --merged main
				Output: "* main\n",
			},
		},
	}

	var buf bytes.Buffer
	err := cleanCommandWithCommandExecutor(&buf, mockExec, "/repo")

	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "No merged worktrees found")
	// list + symbolic-ref + merged = 3 commands
	assert.Len(t, mockExec.executedCommands, 3)
}

// ===== Mock Implementations =====

type mockCleanExecutor struct {
	executedCommands []command.Command
	results          []command.Result
	callCount        int
	failOnCall       int
	failError        string
}

func (m *mockCleanExecutor) Execute(commands []command.Command) (*command.ExecutionResult, error) {
	m.executedCommands = append(m.executedCommands, commands...)

	if m.failError != "" && m.callCount == m.failOnCall {
		m.callCount++
		return nil, &mockCleanError{message: m.failError}
	}

	results := make([]command.Result, len(commands))
	for i, cmd := range commands {
		if m.callCount < len(m.results) {
			results[i] = m.results[m.callCount]
			results[i].Command = cmd
		} else {
			results[i] = command.Result{
				Command: cmd,
				Output:  "",
			}
		}
		m.callCount++
	}

	return &command.ExecutionResult{Results: results}, nil
}

type mockCleanError struct {
	message string
}

func (e *mockCleanError) Error() string {
	return e.message
}
