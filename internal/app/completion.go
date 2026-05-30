package app

import (
	"fmt"
	"io"
)

var completions = map[string]string{
	"bash": `_de_completions() {
    local commands="exec set get unset list init migrate key doctor export completion version help"
    local cur="${COMP_WORDS[COMP_CWORD]}"
    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=($(compgen -W "$commands" -- "$cur"))
    fi
}
complete -F _de_completions de`,

	"zsh": `#compdef de
_de() {
    local -a commands=(exec set get unset list init migrate key doctor export completion version help)
    _arguments '1:command:compadd -a commands' '*::arg:->args'
}
compdef _de de`,

	"fish": `complete -c de -f
complete -c de -n '__fish_use_subcommand' -a 'exec set get unset list init migrate key doctor export completion version help'
complete -c de -n '__fish_seen_subcommand_from get list export' -l reveal -d 'Show values in plaintext'
complete -c de -n '__fish_seen_subcommand_from get list export set unset init migrate' -l dir -d 'Directory context'
complete -c de -n '__fish_seen_subcommand_from export' -l format -a 'dotenv json' -d 'Output format'
complete -c de -n '__fish_seen_subcommand_from migrate' -l file -d 'Env file path'
complete -c de -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish powershell'`,

	"powershell": `Register-ArgumentCompleter -CommandName de -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $commands = @('exec','set','get','unset','list','init','migrate','key','doctor','export','completion','version','help')
    $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}`,
}

func runCompletion(ctx *Context, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(ctx.Err, "usage: de completion <bash|zsh|fish|powershell>")
		return 2
	}

	script, ok := completions[args[0]]
	if !ok {
		fmt.Fprintf(ctx.Err, "unsupported shell: %s (use bash, zsh, fish, or powershell)\n", args[0])
		return 2
	}

	io.WriteString(ctx.Out, script)
	fmt.Fprintln(ctx.Out)
	return 0
}
