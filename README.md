# PCL

[![Built with Grok](https://img.shields.io/badge/Built_with-Grok-000000?style=flat&logo=xai&logoColor=white)](https://grok.com)

A command shell that can **run programs**, **edit in Neovim**, and **talk to a model** in the same session.

Type at a prompt. Pipe output. `cd` around. Ask `p(...)` to read files, write files, and run commands until it has an answer — then open whatever it touched in vim.

```
pcl>
```

---

## Start

```powershell
go build -o pcl.exe ./cmd/pcl
copy config.pcl.example config.pcl
$env:OPENCODE_API_KEY = "your-api-key"
.\pcl.exe
```

No key yet? Offline mock:

```powershell
.\pcl.exe --mock-ai
```

Run a script instead of the REPL: `.\pcl.exe demo.pcl`  
One shot: `.\pcl.exe -c "pwd"`

History lives in `~/.pcl_history` (`history`, `history clear`). **Tab** completes commands, files, `$vars`, and `ai_config` keys.  
`p(...)` / `agent` run **in the background**. Tool use looks like other agents (`● write_file  path`, result underneath); the answer prints as normal text. You can keep typing. `jobs` lists them. Ctrl-C clears the line; it does not kill the job.  
Editor: `nvim` if it’s on `PATH`, or set `PCL_EDITOR` / `EDITOR`.

Startup file: `config.pcl` in the current directory, or `~/.pclrc`.

---

## The shell

Everyday ash-ish commands work here — and on Windows without GNU tools:

```pcl
cd ~/src/app
pwd
ls
ls -l
ls -a
cat README.md | grep TODO
mkdir -p tmp/out
cp README.md tmp/out/
mv tmp/out/README.md tmp/out/notes.md
touch scratch.txt
rm -i scratch.txt          # asks first; rm -f to skip
```

Pipelines and taps:

```pcl
cat access.log | grep "500" | wc -l
cat access.log |> $all | grep "500" |> $errors
echo $errors
echo $status               # last exit code; also $?
```

Files as objects, not `test -f`:

```pcl
if ("config.pcl".exists()) (
    puts "config.pcl".read()
)
"notes.txt".write("hello\n")
"notes.txt".append("more\n")
```

Aliases, glob, env:

```pcl
alias ll="ls"
glob "*.go"
export EDITOR=nvim
cd -
```

Blocks use `( )`. Lists and dicts use `{ }`.

```pcl
i = 1
while ($i <= 3) (
    echo $i
    i = ($i + 1)
)
files = { main.go server.go }
```

Anything that isn’t a builtin is looked up on `PATH` (`grep`, `git`, `go`, …).

### BusyBox (`sh`)

POSIX `sh` is how the **agent `sh` tool** and the **`sh` command** run scripts (`grep`, `sed`, `test`, pipelines the model writes, and so on).

| OS | What PCL runs |
|---|---|
| Windows | `busybox sh -c "…"` |
| Linux | `sh -c "…"` |

On Windows, put **BusyBox** on `PATH` or in the current directory as `busybox.exe` (or `busybox64.exe`). Without it, `sh` fails with “busybox not found”.

A common Windows build is [BusyBox for Windows](https://frippery.org/busybox/) (or any `busybox.exe` that implements the `sh` applet). Check:

```pcl
which busybox
sh -c "echo ok && uname"
```

Linux needs a normal `/bin/sh` (dash or bash in `sh` mode). You do not need BusyBox there.

PCL builtins (`ls`, `cd`, `rm`, `echo`, …) do **not** go through BusyBox. Only `sh` and the agent’s `sh` tool do.

### `$` vs methods

Bare words are **commands**. `$name` is the **value**. Assignment has no `$`.

```pcl
x = p( summarize this repo )
x              # command named x (usually “not found”)
$x             # the response value
echo $x
puts "got $x"
```

Once you use **`.` or `()`**, it is a method on that variable — **no `$`**:

```pcl
x.vim()
x.files()
x.response
"notes.txt".read()
[glob "*.go"].vim()
```

You still need `$` for:

- the value itself: `$x`, `echo $x`, `echo $status` (also `$?`)
- strings: `"hello $name"`
- expressions: `while ($i <= 3)`, `if ($a > $b)`
- pipeline taps: `|> $errors`
- `export` / env names are not `$`: `export EDITOR=nvim`

---

## Prompting

`p( your ask )` is the prompt. The model can call tools (`read_file`, `write_file`, `sh`, plus any `tool` you define). PCL runs them and sends the output back until the model is done.

```pcl
x = p( read config.pcl and explain the model setting )
puts x.response
puts x.reasoning
puts x.tools_used()
```

Stream tokens as they arrive:

```pcl
p!( write a commit message for the last diff )
```

Delimiters: `p( ... )`, `p{ ... }`, `p/ ... /`. `$vars` expand inside.

Open the result in the editor:

```pcl
x = p( add a README section on install )
x.vim()
```

Longer jobs:

```pcl
x = p( fix the failing tests ) with { max_turns: 50 }
agent "inspect the repo and run tests"
```

One completion, no tool loop:

```pcl
p( haiku about shells ) with { agent: false }
```

Define a tool the model can call:

```pcl
tool summarize (path) (
    return [glob $path]
)
x = p( summarize *.go and tell me what the package does )
```

---

## Vim / Neovim

After a prompt, files the model wrote or named are on `x.files()`. `.vim()` opens them in tabs (`nvim -p`).

```pcl
x = p( generate main.go and server.go for a tiny HTTP service )
puts x.files()
x.vim()                  # all generated files, one tab each
x.files().vim()
x.reasoning.vim()        # the thought trace in a buffer
[glob "*.go"].vim()      # any list of paths
```

Same idea without AI: edit what you already have.

```pcl
[glob "pkg/**/*.go"].vim()
```

---

## Config

Copy [config.pcl.example](config.pcl.example) to `config.pcl` (gitignored) or `~/.pclrc`. It is ordinary PCL: `export`, aliases, `ai_config`.

PCL speaks **OpenAI Chat Completions**. Same three knobs for every host: `api_base`, `api_key` (`$ENV` is fine), `model`.

**OpenCode**

```pcl
ai_config api_base "https://opencode.ai/zen/go/v1"
ai_config api_key "$OPENCODE_API_KEY"
ai_config model "deepseek-v4-flash"
```

**OpenAI / Codex** (API chat models, not the Codex CLI)

```pcl
ai_config api_base "https://api.openai.com/v1"
ai_config api_key "$OPENAI_API_KEY"
ai_config model "gpt-4.1"
```

**Grok (xAI)**

```pcl
ai_config api_base "https://api.x.ai/v1"
ai_config api_key "$XAI_API_KEY"
ai_config model "grok-4"
```

**OpenRouter** (Claude, Grok, OpenAI, and others behind one key)

```pcl
ai_config api_base "https://openrouter.ai/api/v1"
ai_config api_key "$OPENROUTER_API_KEY"
ai_config model "anthropic/claude-sonnet-4"
```

Native Anthropic Messages is not implemented; Claude goes through OpenRouter (or any other Chat Completions proxy). Uncomment the matching block in `config.pcl.example`.

---

Grammar, builtins vs `PATH`, environment variables, FFI, and internals: **[SPEC.md](SPEC.md)**.

## License

Copyright (C) 2026 Xyzzy Apps \<xyzzyapps@gmail.com\>.

[PolyForm Noncommercial License 1.0.0](LICENSE) — personal, research, education, hobby. Commercial use needs a separate license.
