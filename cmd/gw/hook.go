package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/urfave/cli/v3"
)

// NewHookCommand creates the hook command definition
func NewHookCommand() *cli.Command {
	return &cli.Command{
		Name:  "hook",
		Usage: "Generate shell hook for cd functionality",
		Description: "Generate shell hook scripts that enable the 'gw cd' command to change directories. " +
			"This provides a seamless navigation experience without needing subshells.\n\n" +
			"To enable the hook, add the following to your shell config:\n" +
			"  Bash (~/.bashrc):         eval \"$(gw hook bash)\"\n" +
			"  Zsh (~/.zshrc):           eval \"$(gw hook zsh)\"\n" +
			"  Fish (~/.config/fish/config.fish): gw hook fish | source",
		Commands: []*cli.Command{
			{
				Name:        "bash",
				Usage:       "Generate bash hook script",
				Description: "Generate bash hook script for cd functionality",
				Action:      hookBash,
			},
			{
				Name:        "zsh",
				Usage:       "Generate zsh hook script",
				Description: "Generate zsh hook script for cd functionality",
				Action:      hookZsh,
			},
			{
				Name:        "fish",
				Usage:       "Generate fish hook script",
				Description: "Generate fish hook script for cd functionality",
				Action:      hookFish,
			},
		},
	}
}

func hookBash(_ context.Context, cmd *cli.Command) error {
	w := cmd.Root().Writer
	if w == nil {
		w = os.Stdout
	}
	return printBashHook(w)
}

func hookZsh(_ context.Context, cmd *cli.Command) error {
	w := cmd.Root().Writer
	if w == nil {
		w = os.Stdout
	}
	return printZshHook(w)
}

func hookFish(_ context.Context, cmd *cli.Command) error {
	w := cmd.Root().Writer
	if w == nil {
		w = os.Stdout
	}
	return printFishHook(w)
}

func printBashHook(w io.Writer) error {
	_, err := fmt.Fprintln(w, `# gw cd command hook for bash
gw() {
    for arg in "$@"; do
        if [[ "$arg" == "--generate-shell-completion" ]]; then
            command gw "$@"
            return $?
        fi
    done
    if [[ "$1" == "cd" ]]; then
        local target_dir
        if [[ -z "$2" ]]; then
            target_dir=$(command gw cd 2>/dev/null)
        else
            target_dir=$(command gw cd "$2" 2>/dev/null)
        fi
        if [[ $? -eq 0 && -n "$target_dir" ]]; then
            cd "$target_dir"
        else
            if [[ -z "$2" ]]; then
                command gw cd
            else
                command gw cd "$2"
            fi
        fi
    else
        command gw "$@"
    fi
}`)

	return err
}

func printZshHook(w io.Writer) error {
	_, err := fmt.Fprintln(w, `# gw cd command hook for zsh
gw() {
    for arg in "$@"; do
        if [[ "$arg" == "--generate-shell-completion" ]]; then
            command gw "$@"
            return $?
        fi
    done
    if [[ "$1" == "cd" ]]; then
        local target_dir
        if [[ -z "$2" ]]; then
            target_dir=$(command gw cd 2>/dev/null)
        else
            target_dir=$(command gw cd "$2" 2>/dev/null)
        fi
        if [[ $? -eq 0 && -n "$target_dir" ]]; then
            cd "$target_dir"
        else
            if [[ -z "$2" ]]; then
                command gw cd
            else
                command gw cd "$2"
            fi
        fi
    else
        command gw "$@"
    fi
}`)

	return err
}

func printFishHook(w io.Writer) error {
	_, err := fmt.Fprintln(w, `# gw cd command hook for fish
function gw
    for arg in $argv
        if test "$arg" = "--generate-shell-completion"
            command gw $argv
            return $status
        end
    end
    if test "$argv[1]" = "cd"
        set -l target_dir
        if test -z "$argv[2]"
            set target_dir (command gw cd 2>/dev/null)
        else
            set target_dir (command gw cd $argv[2] 2>/dev/null)
        end
        if test $status -eq 0 -a -n "$target_dir"
            cd "$target_dir"
        else
            if test -z "$argv[2]"
                command gw cd
            else
                command gw cd $argv[2]
            end
        end
    else
        command gw $argv
    end
end`)

	return err
}
