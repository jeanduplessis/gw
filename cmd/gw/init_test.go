package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/urfave/cli/v3"

	"github.com/jeanduplessis/gw/internal/config"
)

func TestNewInitCommand(t *testing.T) {
	cmd := NewInitCommand()

	assert.NotNil(t, cmd)
	assert.Equal(t, "init", cmd.Name)
	assert.Equal(t, "Initialize configuration file", cmd.Usage)
	assert.NotEmpty(t, cmd.Description)
	assert.NotNil(t, cmd.Action)
}

func TestConfigFileMode(t *testing.T) {
	assert.Equal(t, os.FileMode(0o600), os.FileMode(configFileMode))
}

func TestInitCommand_NotInGitRepo(t *testing.T) {
	// Create a temporary directory that is not a git repo
	tempDir := t.TempDir()
	oldDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldDir) }()
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	app := &cli.Command{
		Commands: []*cli.Command{
			NewInitCommand(),
		},
	}

	ctx := context.Background()
	err = app.Run(ctx, []string{"gw", "init"})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not in a git repository")
}

func TestInitCommand_ConfigAlreadyExists_NonTTY_SkipsConfig(t *testing.T) {
	// Save originals
	originalIsTerminal := isTerminalFunc
	originalGetShell := getShellFunc
	defer func() {
		isTerminalFunc = originalIsTerminal
		getShellFunc = originalGetShell
	}()

	isTerminalFunc = func() bool { return false }
	getShellFunc = func() string { return "" }

	// Create a temporary directory
	tempDir := t.TempDir()

	oldDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldDir) }()
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	// Initialize as a git repository
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	err = gitCmd.Run()
	if err != nil {
		t.Skip("git not available")
	}

	// Create existing config file with known content
	configPath := filepath.Join(tempDir, config.ConfigFileName)
	err = os.WriteFile(configPath, []byte("existing config"), 0644)
	assert.NoError(t, err)

	app := &cli.Command{
		Commands: []*cli.Command{
			NewInitCommand(),
		},
	}

	var buf bytes.Buffer
	app.Writer = &buf

	ctx := context.Background()
	err = app.Run(ctx, []string{"gw", "init"})

	// Should NOT error — it skips config creation in non-TTY mode
	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "existing .gw.yml preserved")

	// Original file should be unchanged
	content, readErr := os.ReadFile(configPath)
	assert.NoError(t, readErr)
	assert.Equal(t, "existing config", string(content))
}

func TestInitCommand_ConfigAlreadyExists_TTY_UserDeclinesOverwrite(t *testing.T) {
	// Save originals
	originalIsTerminal := isTerminalFunc
	originalGetShell := getShellFunc
	defer func() {
		isTerminalFunc = originalIsTerminal
		getShellFunc = originalGetShell
	}()

	isTerminalFunc = func() bool { return true }
	getShellFunc = func() string { return "" }

	// Create a temporary directory
	tempDir := t.TempDir()

	oldDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldDir) }()
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	// Initialize as a git repository
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	err = gitCmd.Run()
	if err != nil {
		t.Skip("git not available")
	}

	// Create existing config file
	configPath := filepath.Join(tempDir, config.ConfigFileName)
	err = os.WriteFile(configPath, []byte("existing config"), 0644)
	assert.NoError(t, err)

	app := &cli.Command{
		Commands: []*cli.Command{
			NewInitCommand(),
		},
	}

	var buf bytes.Buffer
	app.Writer = &buf
	app.Reader = strings.NewReader("n\n")

	ctx := context.Background()
	err = app.Run(ctx, []string{"gw", "init"})

	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Overwrite")
	assert.Contains(t, output, "existing .gw.yml preserved")

	// Original file should be unchanged
	content, readErr := os.ReadFile(configPath)
	assert.NoError(t, readErr)
	assert.Equal(t, "existing config", string(content))
}

func TestInitCommand_ConfigAlreadyExists_TTY_UserAcceptsOverwrite(t *testing.T) {
	// Save originals
	originalIsTerminal := isTerminalFunc
	originalGetShell := getShellFunc
	defer func() {
		isTerminalFunc = originalIsTerminal
		getShellFunc = originalGetShell
	}()

	isTerminalFunc = func() bool { return true }
	getShellFunc = func() string { return "" }

	// Create a temporary directory
	tempDir := t.TempDir()

	oldDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldDir) }()
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	// Initialize as a git repository
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	err = gitCmd.Run()
	if err != nil {
		t.Skip("git not available")
	}

	// Create existing config file with old content
	configPath := filepath.Join(tempDir, config.ConfigFileName)
	err = os.WriteFile(configPath, []byte("existing config"), 0644)
	assert.NoError(t, err)

	app := &cli.Command{
		Commands: []*cli.Command{
			NewInitCommand(),
		},
	}

	var buf bytes.Buffer
	app.Writer = &buf
	app.Reader = strings.NewReader("y\n")

	ctx := context.Background()
	err = app.Run(ctx, []string{"gw", "init"})

	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Overwrite")
	assert.Contains(t, output, "Configuration file created:")

	// File should be overwritten with new config
	content, readErr := os.ReadFile(configPath)
	assert.NoError(t, readErr)
	assert.Contains(t, string(content), `version: "1.0"`)
	assert.NotEqual(t, "existing config", string(content))
}

func TestInitCommand_ConfigAlreadyExists_TTY_EmptyInputDeclinesOverwrite(t *testing.T) {
	// Save originals
	originalIsTerminal := isTerminalFunc
	originalGetShell := getShellFunc
	defer func() {
		isTerminalFunc = originalIsTerminal
		getShellFunc = originalGetShell
	}()

	isTerminalFunc = func() bool { return true }
	getShellFunc = func() string { return "" }

	// Create a temporary directory
	tempDir := t.TempDir()

	oldDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldDir) }()
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	// Initialize as a git repository
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	err = gitCmd.Run()
	if err != nil {
		t.Skip("git not available")
	}

	// Create existing config file
	configPath := filepath.Join(tempDir, config.ConfigFileName)
	err = os.WriteFile(configPath, []byte("existing config"), 0644)
	assert.NoError(t, err)

	app := &cli.Command{
		Commands: []*cli.Command{
			NewInitCommand(),
		},
	}

	var buf bytes.Buffer
	app.Writer = &buf
	app.Reader = strings.NewReader("\n")

	ctx := context.Background()
	err = app.Run(ctx, []string{"gw", "init"})

	assert.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "existing .gw.yml preserved")

	// File unchanged
	content, readErr := os.ReadFile(configPath)
	assert.NoError(t, readErr)
	assert.Equal(t, "existing config", string(content))
}

func TestInitCommand_Success(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	oldDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldDir) }()
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	// Initialize as a git repository
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	err = gitCmd.Run()
	if err != nil {
		t.Skip("git not available")
	}

	app := &cli.Command{
		Commands: []*cli.Command{
			NewInitCommand(),
		},
	}

	var buf bytes.Buffer
	app.Writer = &buf

	ctx := context.Background()
	err = app.Run(ctx, []string{"gw", "init"})
	assert.NoError(t, err)

	// Check output
	output := buf.String()
	assert.Contains(t, output, "Configuration file created:")
	assert.Contains(t, output, config.ConfigFileName)
	assert.Contains(t, output, "Edit this file to customize your worktree setup.")

	// Verify config file was created
	configPath := filepath.Join(tempDir, config.ConfigFileName)
	info, err := os.Stat(configPath)
	assert.NoError(t, err)
	assert.False(t, info.IsDir())

	// Check file permissions
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// Verify content
	content, err := os.ReadFile(configPath)
	assert.NoError(t, err)
	contentStr := string(content)

	// Check for required sections
	assert.Contains(t, contentStr, "version: \"1.0\"")
	assert.Contains(t, contentStr, "defaults:")
	assert.Contains(t, contentStr, "base_dir: ../worktrees")
	assert.Contains(t, contentStr, "hooks:")
	assert.Contains(t, contentStr, "post_create:")

	// Check for example hooks
	assert.Contains(t, contentStr, "type: copy")
	assert.Contains(t, contentStr, "from: .env")
	assert.Contains(t, contentStr, "to: .env")
	assert.Contains(t, contentStr, "type: command")
	assert.Contains(t, contentStr, "command: gw")
	assert.Contains(t, contentStr, `command: gw list`)

	// Check for comments
	assert.Contains(t, contentStr, "# gw Configuration")
	assert.Contains(t, contentStr, "# Default settings for worktrees")
	assert.Contains(t, contentStr, "# Hooks that run after creating a worktree")
}

func TestInitCommand_DirectoryAccessError(t *testing.T) {
	// Save original os.Getwd to restore later
	originalGetwd := osGetwd
	defer func() { osGetwd = originalGetwd }()

	// Mock os.Getwd to return an error
	osGetwd = func() (string, error) {
		return "", assert.AnError
	}

	cmd := NewInitCommand()
	ctx := context.Background()
	err := cmd.Action(ctx, &cli.Command{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to access")
}

func TestInitCommand_IncludesShellSetupPrompt(t *testing.T) {
	// Save all mockable functions
	originalGetShell := getShellFunc
	originalIsTerminal := isTerminalFunc
	originalGetHome := getUserHomeDir
	originalReadFile := readFileFunc
	defer func() {
		getShellFunc = originalGetShell
		isTerminalFunc = originalIsTerminal
		getUserHomeDir = originalGetHome
		readFileFunc = originalReadFile
	}()

	// Configure mocks: detected shell, not a TTY, no existing integration
	getShellFunc = func() string { return "zsh" }
	isTerminalFunc = func() bool { return false }
	getUserHomeDir = func() (string, error) { return "/home/test", nil }
	readFileFunc = func(_ string) ([]byte, error) { return nil, os.ErrNotExist }

	// Create a temporary directory
	tempDir := t.TempDir()

	oldDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldDir) }()
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	// Initialize as a git repository
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	err = gitCmd.Run()
	if err != nil {
		t.Skip("git not available")
	}

	app := &cli.Command{
		Commands: []*cli.Command{
			NewInitCommand(),
		},
	}

	var buf bytes.Buffer
	app.Writer = &buf

	ctx := context.Background()
	err = app.Run(ctx, []string{"gw", "init"})
	assert.NoError(t, err)

	output := buf.String()
	// Should contain both the config creation message and shell integration info
	assert.Contains(t, output, "Configuration file created:")
	assert.Contains(t, output, "gw shell-init zsh")
}

func TestInitCommand_WriteFileError(t *testing.T) {
	// Create a temporary directory
	tempDir := t.TempDir()

	oldDir, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldDir) }()
	err := os.Chdir(tempDir)
	assert.NoError(t, err)

	originalWriteFile := writeFile
	writeFile = func(string, []byte, os.FileMode) error {
		return assert.AnError
	}
	defer func() { writeFile = originalWriteFile }()

	// Initialize as a git repository
	gitCmd := exec.Command("git", "init")
	gitCmd.Dir = tempDir
	err = gitCmd.Run()
	if err != nil {
		t.Skip("git not available")
	}

	cmd := NewInitCommand()
	ctx := context.Background()
	err = cmd.Action(ctx, &cli.Command{})

	assert.Error(t, err)
	// The error could be either "failed to access" or "failed to create"
	errorMsg := err.Error()
	assert.True(t, strings.Contains(errorMsg, "failed to") &&
		(strings.Contains(errorMsg, "access") || strings.Contains(errorMsg, "create")))
}
