package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectShell(t *testing.T) {
	tests := []struct {
		name     string
		envShell string
		expected string
	}{
		{"zsh", "/bin/zsh", "zsh"},
		{"bash", "/bin/bash", "bash"},
		{"fish", "/usr/local/bin/fish", "fish"},
		{"empty", "", ""},
		{"unsupported", "/bin/csh", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := os.Getenv("SHELL")
			defer func() { _ = os.Setenv("SHELL", old) }()
			_ = os.Setenv("SHELL", tt.envShell)

			result := detectShell()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShellInitLine(t *testing.T) {
	tests := []struct {
		shell    string
		expected string
	}{
		{"bash", `eval "$(gw shell-init bash)"`},
		{"zsh", `eval "$(gw shell-init zsh)"`},
		{"fish", "gw shell-init fish | source"},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			assert.Equal(t, tt.expected, shellInitLine(tt.shell))
		})
	}
}

func TestShellConfigPath(t *testing.T) {
	originalGetHome := getUserHomeDir
	defer func() { getUserHomeDir = originalGetHome }()

	getUserHomeDir = func() (string, error) {
		return "/home/testuser", nil
	}

	tests := []struct {
		shell    string
		expected string
		hasError bool
	}{
		{"bash", "/home/testuser/.bashrc", false},
		{"zsh", "/home/testuser/.zshrc", false},
		{"fish", "/home/testuser/.config/fish/config.fish", false},
		{"csh", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			path, err := shellConfigPath(tt.shell)
			if tt.hasError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, path)
			}
		})
	}
}

func TestShellIntegrationExists(t *testing.T) {
	originalReadFile := readFileFunc
	defer func() { readFileFunc = originalReadFile }()

	t.Run("file contains gw shell-init", func(t *testing.T) {
		readFileFunc = func(_ string) ([]byte, error) {
			return []byte(`# my config
eval "$(gw shell-init zsh)"
`), nil
		}
		exists, err := shellIntegrationExists("/fake/path")
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("file contains gw hook", func(t *testing.T) {
		readFileFunc = func(_ string) ([]byte, error) {
			return []byte(`# my config
eval "$(gw hook zsh)"
`), nil
		}
		exists, err := shellIntegrationExists("/fake/path")
		assert.NoError(t, err)
		assert.True(t, exists)
	})

	t.Run("file does not contain integration", func(t *testing.T) {
		readFileFunc = func(_ string) ([]byte, error) {
			return []byte("# just some config\nexport PATH=$PATH\n"), nil
		}
		exists, err := shellIntegrationExists("/fake/path")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("file does not exist", func(t *testing.T) {
		readFileFunc = func(_ string) ([]byte, error) {
			return nil, os.ErrNotExist
		}
		exists, err := shellIntegrationExists("/fake/path")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("file read error", func(t *testing.T) {
		readFileFunc = func(_ string) ([]byte, error) {
			return nil, os.ErrPermission
		}
		_, err := shellIntegrationExists("/fake/path")
		assert.Error(t, err)
	})
}

func TestAppendToFile(t *testing.T) {
	t.Run("creates file and directories", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := filepath.Join(tmpDir, "sub", "dir", "config")

		err := appendToFile(target, "hello\n")
		require.NoError(t, err)

		content, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "hello\n", string(content))
	})

	t.Run("appends to existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		target := filepath.Join(tmpDir, "config")
		require.NoError(t, os.WriteFile(target, []byte("existing\n"), 0o644))

		err := appendToFile(target, "appended\n")
		require.NoError(t, err)

		content, err := os.ReadFile(target)
		require.NoError(t, err)
		assert.Equal(t, "existing\nappended\n", string(content))
	})
}

func TestPromptShellSetup(t *testing.T) {
	// Save originals
	originalGetShell := getShellFunc
	originalIsTerminal := isTerminalFunc
	originalGetHome := getUserHomeDir
	originalReadFile := readFileFunc
	originalAppendFile := appendFileFunc

	defer func() {
		getShellFunc = originalGetShell
		isTerminalFunc = originalIsTerminal
		getUserHomeDir = originalGetHome
		readFileFunc = originalReadFile
		appendFileFunc = originalAppendFile
	}()

	t.Run("unknown shell prints generic instructions", func(t *testing.T) {
		getShellFunc = func() string { return "" }

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader(""))
		assert.NoError(t, err)
		assert.False(t, added)
		assert.Contains(t, out.String(), "gw shell-init bash")
		assert.Contains(t, out.String(), "gw shell-init zsh")
		assert.Contains(t, out.String(), "gw shell-init fish")
	})

	t.Run("already configured shows message", func(t *testing.T) {
		getShellFunc = func() string { return "zsh" }
		getUserHomeDir = func() (string, error) { return "/home/test", nil }
		readFileFunc = func(_ string) ([]byte, error) {
			return []byte(`eval "$(gw shell-init zsh)"`), nil
		}

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader(""))
		assert.NoError(t, err)
		assert.False(t, added)
		assert.Contains(t, out.String(), "already configured")
	})

	t.Run("non-TTY prints instructions without prompt", func(t *testing.T) {
		getShellFunc = func() string { return "zsh" }
		getUserHomeDir = func() (string, error) { return "/home/test", nil }
		readFileFunc = func(_ string) ([]byte, error) { return nil, os.ErrNotExist }
		isTerminalFunc = func() bool { return false }

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader(""))
		assert.NoError(t, err)
		assert.False(t, added)
		assert.Contains(t, out.String(), "gw shell-init zsh")
		assert.Contains(t, out.String(), ".zshrc")
		// Should NOT contain the interactive prompt
		assert.NotContains(t, out.String(), "[y/N]")
	})

	t.Run("user declines prompt", func(t *testing.T) {
		getShellFunc = func() string { return "bash" }
		getUserHomeDir = func() (string, error) { return "/home/test", nil }
		readFileFunc = func(_ string) ([]byte, error) { return nil, os.ErrNotExist }
		isTerminalFunc = func() bool { return true }

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader("n\n"))
		assert.NoError(t, err)
		assert.False(t, added)
		assert.Contains(t, out.String(), "[y/N]")
		assert.Contains(t, out.String(), "Skipped")
	})

	t.Run("user accepts prompt", func(t *testing.T) {
		getShellFunc = func() string { return "zsh" }
		getUserHomeDir = func() (string, error) { return "/home/test", nil }
		readFileFunc = func(_ string) ([]byte, error) { return nil, os.ErrNotExist }
		isTerminalFunc = func() bool { return true }

		var appendedPath, appendedText string
		appendFileFunc = func(path, text string) error {
			appendedPath = path
			appendedText = text
			return nil
		}

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader("y\n"))
		assert.NoError(t, err)
		assert.True(t, added)
		assert.Equal(t, "/home/test/.zshrc", appendedPath)
		assert.Contains(t, appendedText, "gw shell-init zsh")
		assert.Contains(t, appendedText, "# gw shell integration")
		assert.Contains(t, out.String(), "Shell integration added")
		assert.Contains(t, out.String(), "source")
	})

	t.Run("user types yes", func(t *testing.T) {
		getShellFunc = func() string { return "bash" }
		getUserHomeDir = func() (string, error) { return "/home/test", nil }
		readFileFunc = func(_ string) ([]byte, error) { return nil, os.ErrNotExist }
		isTerminalFunc = func() bool { return true }
		appendFileFunc = func(_, _ string) error { return nil }

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader("yes\n"))
		assert.NoError(t, err)
		assert.True(t, added)
	})

	t.Run("empty input declines", func(t *testing.T) {
		getShellFunc = func() string { return "zsh" }
		getUserHomeDir = func() (string, error) { return "/home/test", nil }
		readFileFunc = func(_ string) ([]byte, error) { return nil, os.ErrNotExist }
		isTerminalFunc = func() bool { return true }

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader("\n"))
		assert.NoError(t, err)
		assert.False(t, added)
		assert.Contains(t, out.String(), "Skipped")
	})

	t.Run("append failure shows manual instructions", func(t *testing.T) {
		getShellFunc = func() string { return "zsh" }
		getUserHomeDir = func() (string, error) { return "/home/test", nil }
		readFileFunc = func(_ string) ([]byte, error) { return nil, os.ErrNotExist }
		isTerminalFunc = func() bool { return true }
		appendFileFunc = func(_, _ string) error { return os.ErrPermission }

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader("y\n"))
		assert.NoError(t, err)
		assert.False(t, added)
		assert.Contains(t, out.String(), "Failed to update")
		assert.Contains(t, out.String(), "add it manually")
	})

	t.Run("fish shell uses correct syntax", func(t *testing.T) {
		getShellFunc = func() string { return "fish" }
		getUserHomeDir = func() (string, error) { return "/home/test", nil }
		readFileFunc = func(_ string) ([]byte, error) { return nil, os.ErrNotExist }
		isTerminalFunc = func() bool { return true }

		var appendedText string
		appendFileFunc = func(_, text string) error {
			appendedText = text
			return nil
		}

		var out bytes.Buffer
		added, err := promptShellSetup(&out, strings.NewReader("y\n"))
		assert.NoError(t, err)
		assert.True(t, added)
		assert.Contains(t, appendedText, "gw shell-init fish | source")
		assert.NotContains(t, appendedText, "eval")
	})
}
