package e2e

import (
	"strings"
	"testing"

	"github.com/jeanduplessis/gw/test/e2e/framework"
)

func TestClean(t *testing.T) {
	env := framework.NewTestEnvironment(t)
	defer env.Cleanup()

	t.Run("RemovesMergedWorktree", func(t *testing.T) {
		repo := env.CreateTestRepo("clean-merged")

		// Create a feature branch and worktree
		repo.CreateBranch("feature/done")
		_, err := repo.RunWTP("add", "feature/done")
		framework.AssertNoError(t, err)
		framework.AssertWorktreeCount(t, repo, 2)

		// Merge the feature branch into main (it points to the same commit, so it's already merged)
		// Since CreateBranch creates from HEAD, it's already merged by default.

		// Run clean
		output, err := repo.RunWTP("clean")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "Removed worktree")
		framework.AssertOutputContains(t, output, "feature/done")
		framework.AssertOutputContains(t, output, "Removed branch")

		// Verify worktree was removed
		framework.AssertWorktreeCount(t, repo, 1)
	})

	t.Run("SkipsUnmergedWorktree", func(t *testing.T) {
		repo := env.CreateTestRepo("clean-unmerged")

		// Create a feature branch, add a worktree, and make a commit so it's NOT merged
		repo.CreateBranch("feature/wip")
		_, err := repo.RunWTP("add", "feature/wip")
		framework.AssertNoError(t, err)

		// Find the worktree path and add a commit to it
		worktrees := repo.ListWorktrees()
		var worktreePath string
		for _, wt := range worktrees {
			if strings.Contains(wt, "feature/wip") {
				worktreePath = wt
				break
			}
		}

		if worktreePath != "" {
			// Make a commit in the feature worktree so it diverges from main
			env.WriteFile(worktreePath+"/new-feature.txt", "new content")
			env.RunInDir(worktreePath, "git", "add", ".")
			env.RunInDir(worktreePath, "git", "commit", "-m", "feat: add new feature")
		}

		framework.AssertWorktreeCount(t, repo, 2)

		// Run clean
		output, err := repo.RunWTP("clean")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "No merged worktrees found")

		// Verify worktree is still there
		framework.AssertWorktreeCount(t, repo, 2)
	})

	t.Run("NoExtraWorktrees", func(t *testing.T) {
		repo := env.CreateTestRepo("clean-empty")

		output, err := repo.RunWTP("clean")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "No merged worktrees found")
	})

	t.Run("MixedMergedAndUnmerged", func(t *testing.T) {
		repo := env.CreateTestRepo("clean-mixed")

		// Create a merged branch (same commit as main)
		repo.CreateBranch("feature/merged")
		_, err := repo.RunWTP("add", "feature/merged")
		framework.AssertNoError(t, err)

		// Create an unmerged branch with new commit
		repo.CreateBranch("feature/unmerged")
		_, err = repo.RunWTP("add", "feature/unmerged")
		framework.AssertNoError(t, err)

		// Find the unmerged worktree and add a commit
		worktrees := repo.ListWorktrees()
		for _, wt := range worktrees {
			if strings.Contains(wt, "feature/unmerged") {
				env.WriteFile(wt+"/diverge.txt", "diverging content")
				env.RunInDir(wt, "git", "add", ".")
				env.RunInDir(wt, "git", "commit", "-m", "feat: diverge from main")
				break
			}
		}

		framework.AssertWorktreeCount(t, repo, 3)

		// Run clean
		output, err := repo.RunWTP("clean")
		framework.AssertNoError(t, err)

		// The merged worktree should be removed
		framework.AssertOutputContains(t, output, "feature/merged")
		framework.AssertOutputContains(t, output, "Removed worktree")

		// The unmerged worktree should still exist
		framework.AssertWorktreeCount(t, repo, 2)
		framework.AssertWorktreeExists(t, repo, "feature/unmerged")
	})

	t.Run("MergedViaMergeCommit", func(t *testing.T) {
		repo := env.CreateTestRepo("clean-merge-commit")

		// Create a feature branch with a new commit
		repo.CreateBranch("feature/to-merge")
		_, err := repo.RunWTP("add", "feature/to-merge")
		framework.AssertNoError(t, err)

		// Add a commit to the feature branch
		worktrees := repo.ListWorktrees()
		var featurePath string
		for _, wt := range worktrees {
			if strings.Contains(wt, "feature/to-merge") {
				featurePath = wt
				break
			}
		}
		if featurePath != "" {
			env.WriteFile(featurePath+"/feature.txt", "feature content")
			env.RunInDir(featurePath, "git", "add", ".")
			env.RunInDir(featurePath, "git", "commit", "-m", "feat: feature work")
		}

		// Merge the feature branch into main from the main worktree
		env.RunInDir(repo.Path(), "git", "merge", "feature/to-merge", "--no-ff", "-m", "merge feature")

		framework.AssertWorktreeCount(t, repo, 2)

		// Run clean - the feature branch is now merged into main
		output, err := repo.RunWTP("clean")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "Removed worktree")
		framework.AssertOutputContains(t, output, "feature/to-merge")

		framework.AssertWorktreeCount(t, repo, 1)
	})
}
