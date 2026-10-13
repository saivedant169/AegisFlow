package main

import (
	"fmt"
	"strings"
)

var completionCommands = []string{
	"pending", "approve", "deny", "verify", "evidence", "simulate", "why",
	"diff-policy", "manifest", "supply-chain", "policy-pack", "test-action",
	"plugin", "status", "usage", "models", "providers", "policies", "tenants",
	"policy", "test", "version", "completion", "help",
}

func bashCompletionScript() string {
	return `_aegisctl_completion() {
	local cur commands
	COMPREPLY=()
	cur="${COMP_WORDS[COMP_CWORD]}"
	commands="` + strings.Join(completionCommands, " ") + `"
	if [ "$COMP_CWORD" -eq 1 ]; then
		COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
	fi
}
complete -F _aegisctl_completion aegisctl
`
}

func zshCompletionScript() string {
	return `#compdef aegisctl

_aegisctl() {
	(( CURRENT == 2 )) || return 1
	local -a commands
	commands=(` + strings.Join(completionCommands, " ") + `)
	_describe 'command' commands
}
# When autoloaded from fpath, complete on the first invocation too.
if [[ $funcstack[1] == _aegisctl ]]; then
	_aegisctl "$@"
else
	compdef _aegisctl aegisctl
fi
`
}

func cmdCompletion(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: aegisctl completion <bash|zsh>")
	}
	var script string
	switch args[0] {
	case "bash":
		script = bashCompletionScript()
	case "zsh":
		script = zshCompletionScript()
	default:
		return fmt.Errorf("unknown shell %q, expected \"bash\" or \"zsh\"", args[0])
	}
	if _, err := fmt.Print(script); err != nil {
		return fmt.Errorf("could not write completion script: %w", err)
	}
	return nil
}
