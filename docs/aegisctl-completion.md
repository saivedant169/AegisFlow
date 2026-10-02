# aegisctl Shell Completion

`aegisctl` can print command completion scripts for Bash and Zsh:

```bash
aegisctl completion bash
aegisctl completion zsh
```

Both scripts complete top-level command names. Subcommands, flags, and argument
values are not completed.

## Bash Installation

With `aegisctl` on your `PATH`, load completions in the current shell with:

```bash
eval "$(aegisctl completion bash)"
```

Add that line to `~/.bashrc` to load completions in future Bash sessions.

## Zsh Installation

With `aegisctl` on your `PATH`, add this to `~/.zshrc`:

```zsh
autoload -Uz compinit
compinit
source <(aegisctl completion zsh)
```

If your Zsh configuration already runs `compinit`, add only the `source` line
after it.
