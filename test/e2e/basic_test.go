package e2e

import (
	"strings"
	"testing"

	"github.com/jeanduplessis/gw/test/e2e/framework"
)

func TestBasicCommands(t *testing.T) {
	env := framework.NewTestEnvironment(t)
	defer env.Cleanup()

	t.Run("Version", func(t *testing.T) {
		output, err := env.RunWTP("--version")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "gw version")
	})

	t.Run("Help", func(t *testing.T) {
		output, err := env.RunWTP("--help")
		framework.AssertNoError(t, err)

		expectedCommands := []string{"add", "remove", "list", "init", "cd"}
		framework.AssertMultipleStringsInOutput(t, output, expectedCommands)

		framework.AssertOutputContains(t, output, "USAGE:")
		framework.AssertOutputContains(t, output, "COMMANDS:")
		framework.AssertOutputContains(t, output, "GLOBAL OPTIONS:")
	})

	t.Run("HelpForCommand", func(t *testing.T) {
		commands := []string{"add", "remove", "list", "init", "cd"}

		for _, cmd := range commands {
			output, err := env.RunWTP(cmd, "--help")
			framework.AssertNoError(t, err)
			framework.AssertOutputContains(t, output, "USAGE:")
			framework.AssertOutputContains(t, output, cmd)
		}
	})
}

func TestInitCommand(t *testing.T) {
	env := framework.NewTestEnvironment(t)
	defer env.Cleanup()

	t.Run("CreateConfig", func(t *testing.T) {
		repo := env.CreateTestRepo("init-test")

		output, err := repo.RunWTP("init")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "Configuration file created")
		framework.AssertFileExists(t, repo, ".gw.yml")

		content := repo.ReadFile(".gw.yml")
		framework.AssertTrue(t, strings.Contains(content, "version:"), "Config should contain version")
		framework.AssertTrue(t, strings.Contains(content, "base_dir:"), "Config should contain base_dir")
	})

	t.Run("ConfigAlreadyExists_SkipsInNonTTY", func(t *testing.T) {
		repo := env.CreateTestRepo("init-exists-test")

		_, err := repo.RunWTP("init")
		framework.AssertNoError(t, err)

		// Second init in non-TTY mode should succeed (skip config creation)
		output, err := repo.RunWTP("init")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "existing .gw.yml preserved")
	})

	t.Run("InitShowsShellIntegrationInfo", func(t *testing.T) {
		repo := env.CreateTestRepo("init-shell-test")

		output, err := repo.RunWTP("init")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "Configuration file created")
		// Non-interactive (E2E runs are not TTY), so should show shell integration instructions
		framework.AssertTrue(t,
			strings.Contains(output, "shell-init") ||
				strings.Contains(output, "shell integration"),
			"Init output should mention shell integration setup")
	})

	t.Run("InitOutsideRepo", func(t *testing.T) {
		cmd := env.CreateNonRepoDir("not-a-repo")

		output, err := cmd.RunWTP("init")
		framework.AssertError(t, err)
		framework.AssertOutputContains(t, output, "not in a git repository")
		framework.AssertHelpfulError(t, output)
	})
}

func TestVersionCommand(t *testing.T) {
	env := framework.NewTestEnvironment(t)
	defer env.Cleanup()

	t.Run("ShortFlag", func(t *testing.T) {
		output, err := env.RunWTP("-v")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "gw version")
	})

	t.Run("LongFlag", func(t *testing.T) {
		output, err := env.RunWTP("--version")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "gw version")
	})
}

func TestHelpCommand(t *testing.T) {
	env := framework.NewTestEnvironment(t)
	defer env.Cleanup()

	t.Run("ShortFlag", func(t *testing.T) {
		output, err := env.RunWTP("-h")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "USAGE:")
	})

	t.Run("LongFlag", func(t *testing.T) {
		output, err := env.RunWTP("--help")
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "USAGE:")
	})

	t.Run("NoArgs", func(t *testing.T) {
		output, err := env.RunWTP()
		framework.AssertNoError(t, err)
		framework.AssertOutputContains(t, output, "USAGE:")
	})
}

func TestCommandsOutsideRepo(t *testing.T) {
	env := framework.NewTestEnvironment(t)
	defer env.Cleanup()

	cmd := env.CreateNonRepoDir("not-a-repo")

	commands := []struct {
		name string
		args []string
	}{
		{"add", []string{"add", "branch"}},
		{"remove", []string{"remove", "branch"}},
		{"list", []string{"list"}},
	}

	for _, tc := range commands {
		t.Run(tc.name, func(t *testing.T) {
			output, err := cmd.RunWTP(tc.args...)
			framework.AssertError(t, err)
			framework.AssertOutputContains(t, output, "not in a git repository")
			framework.AssertHelpfulError(t, output)
		})
	}
}
