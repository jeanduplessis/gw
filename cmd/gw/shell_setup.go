package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

// shellConfig maps shell names to their config file paths (relative to home directory).
var shellConfigs = map[string]string{
	"bash": ".bashrc",
	"zsh":  ".zshrc",
	"fish": ".config/fish/config.fish",
}

// Mockable functions for testing.
var (
	getUserHomeDir = os.UserHomeDir
	readFileFunc   = os.ReadFile
	appendFileFunc = appendToFile
	isTerminalFunc = func() bool {
		return term.IsTerminal(int(os.Stdin.Fd()))
	}
	getShellFunc = detectShell
)

// shellInitLine returns the eval line that should be added to the shell config.
func shellInitLine(shell string) string {
	switch shell {
	case "fish":
		return "gw shell-init fish | source"
	default:
		return fmt.Sprintf(`eval "$(gw shell-init %s)"`, shell)
	}
}

// detectShell returns the current shell name (bash, zsh, or fish).
func detectShell() string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		return ""
	}
	base := filepath.Base(shell)
	if _, ok := shellConfigs[base]; ok {
		return base
	}
	return ""
}

// shellConfigPath returns the absolute path to the shell config file.
func shellConfigPath(shell string) (string, error) {
	home, err := getUserHomeDir()
	if err != nil {
		return "", fmt.Errorf("could not determine home directory: %w", err)
	}

	relPath, ok := shellConfigs[shell]
	if !ok {
		return "", fmt.Errorf("unsupported shell: %s", shell)
	}

	return filepath.Join(home, relPath), nil
}

// shellIntegrationExists checks if the shell config file already contains gw shell-init.
func shellIntegrationExists(configPath string) (bool, error) {
	content, err := readFileFunc(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	return strings.Contains(string(content), "gw shell-init") ||
		strings.Contains(string(content), "gw hook"), nil
}

const (
	shellConfigDirPerm  = 0o750
	shellConfigFilePerm = 0o600
)

// appendToFile appends text to a file, creating parent directories if needed.
func appendToFile(path, text string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, shellConfigDirPerm); err != nil {
		return err
	}

	cleanPath := filepath.Clean(path)
	// #nosec G304 -- path is derived from known shell config paths via shellConfigPath
	f, err := os.OpenFile(cleanPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, shellConfigFilePerm)
	if err != nil {
		return err
	}

	_, writeErr := f.WriteString(text)
	if closeErr := f.Close(); closeErr != nil && writeErr == nil {
		return closeErr
	}
	return writeErr
}

// promptShellSetup handles the interactive shell integration setup after init.
// It detects the shell, checks if integration already exists, and offers to add it.
// Returns true if shell integration was added, false otherwise.
func promptShellSetup(w io.Writer, r io.Reader) (bool, error) {
	shell := getShellFunc()
	if shell == "" {
		return printGenericShellInstructions(w)
	}

	configPath, pathErr := shellConfigPath(shell)
	if pathErr != nil {
		return false, nil
	}

	exists, checkErr := shellIntegrationExists(configPath)
	if checkErr != nil {
		return false, nil
	}

	if exists {
		_, printErr := fmt.Fprintf(w,
			"\nShell integration already configured in %s\n", configPath)
		return false, printErr
	}

	initLine := shellInitLine(shell)

	if !isTerminalFunc() {
		_, printErr := fmt.Fprintf(w,
			"\nTo enable shell integration, add this to %s:\n  %s\n",
			configPath, initLine,
		)
		return false, printErr
	}

	return runInteractiveSetup(w, r, shell, configPath, initLine)
}

// printGenericShellInstructions prints setup instructions for all shells.
func printGenericShellInstructions(w io.Writer) (bool, error) {
	msg := "\nTo enable shell integration (cd and tab completion), " +
		"add to your shell config:\n" +
		"  Bash: eval \"$(gw shell-init bash)\"\n" +
		"  Zsh:  eval \"$(gw shell-init zsh)\"\n" +
		"  Fish: gw shell-init fish | source\n"
	_, err := fmt.Fprint(w, msg)
	return false, err
}

// runInteractiveSetup prompts the user and appends shell integration if accepted.
func runInteractiveSetup(
	w io.Writer, r io.Reader,
	shell, configPath, initLine string,
) (bool, error) {
	prompt := fmt.Sprintf(
		"\nSet up shell integration for %s? "+
			"This enables 'gw cd' and tab completion.\n"+
			"The following will be added to %s:\n  %s\n\n"+
			"Add it now? [y/N] ",
		shell, configPath, initLine,
	)
	if _, printErr := fmt.Fprint(w, prompt); printErr != nil {
		return false, printErr
	}

	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return false, nil
	}

	answer := strings.TrimSpace(strings.ToLower(scanner.Text()))
	if answer != "y" && answer != "yes" {
		_, printErr := fmt.Fprintln(w,
			"\nSkipped. You can set this up later by running:"+
				" gw shell-init --help")
		return false, printErr
	}

	textToAppend := fmt.Sprintf("\n# gw shell integration\n%s\n", initLine)
	if appendErr := appendFileFunc(configPath, textToAppend); appendErr != nil {
		_, _ = fmt.Fprintf(w, "\nFailed to update %s: %v\n",
			configPath, appendErr)
		_, printErr := fmt.Fprintf(w,
			"You can add it manually:\n  %s\n", initLine)
		return false, printErr
	}

	_, printErr := fmt.Fprintf(w,
		"\nShell integration added to %s\n"+
			"Restart your shell or run: source %s\n",
		configPath, configPath)
	return true, printErr
}
