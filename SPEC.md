# Software Requirements Specification — PCL (Prompt Command Language)

## 1. Product identity

PCL is an embeddable command language and interactive shell written in Go. It combines:

- Tcl-style **commands, `proc`, and string-centric values**
- A **portable subset of ash/sh** builtins (native in-process; not BusyBox)
- Perl-style quoting (`p`, `q`, `qq`, `qx`, `qw`)
- Uniform Function Call Syntax (UFCS)
- Optional AI prompts and a ReAct agent loop

PCL is **not** POSIX `sh`. Scripts that assume `&&` / `||` command lists, job control, `test`/`[`, or here-documents are out of scope unless listed below.

End-user usage (REPL, prompts, vim): **[README.md](README.md)**. Interactive REPL uses `github.com/chzyer/readline` (Tab completion, history). `p(...)` / `agent` are background jobs with streaming (`jobs` lists them).

---

## 1.1 Building and tests

```
go build -o pcl.exe ./cmd/pcl
go test -v ./tests/...
pcl --mock-ai demo.pcl
pcl -c "pwd"
```

Flags: `-c` command, `-v` version, `-d` debug, `--mock-ai`, `--config path`.

---

## 2. Architecture

Feature code lives in vertical packages (`pkg/builtins/core.go`, `shell.go`, `ai.go`, `ffi.go`, `pkg/interp/ufcs_eval.go`). The **interpreter** (`pkg/interp/interp.go`) is the single command dispatcher:

1. Expand **aliases**
2. Run a **builtin**
3. Run a **proc**
4. Call a **Go FFI** symbol
5. Else run an **external** program via `ProcessService` (`PATH`)

Subsystems do not call each other except through the interpreter and the service locator (`IOService`, `FSService`, `ProcessService`, `AIService`, `ConfigService`).

ReAct agent loop: `pkg/ai/agent.go` (Reason → Act → Observe → Reflect), tools `sh` / `read_file` / `write_file`.

---

## 3. Native builtins vs external commands

| Native (always available) | Delegated to OS `PATH` |
|---|---|
| Language: `set`, `unset`, `proc`, `return`, `if`, `while`, `for`, `foreach`, `break`, `continue`, `expr`, `source`, `puts`, `gets`, `list`, `lindex`, `llength`, `lappend` | `ls`, `cat`, `grep`, `sed`, `awk`, `head`, `tail`, `wc`, compilers, editors, … |
| Shell: `true`, `false`, `cd`, `pwd`, `echo`, `export`, `env`, `unsetenv`, `exit`, `which`, `clear`/`cls`, `history`, `glob`/`g`, `alias`, `unalias` | Anything not registered as a builtin or FFI name |
| Files: `touch`, `mkdir`, `rmdir`, `rm`, `mv`, `cp`, `ln` | |
| AI: `prompt`/`p`, `agent`, `tool`, `ai_config`, `tools`, `skills`, `mcp` | |
| FFI: `ffi::call`, `ffi::bind`, `ffi::list`, `ffi::load_dll`, `load_go` | |

Windows: prefer native `mv`/`cp`/`ln`/`mkdir`/`rm`; do not assume GNU coreutils.

**OS:** Linux and Windows are first-class. `PATH` lookup uses `exec.LookPath` (Unix executable bit; Windows `PATHEXT`). REPL VT/UTF-8 setup is Windows-only (`pkg/repl/terminal_*.go`). `ffi::load_dll` is Windows-only; other FFI (`load_go`, `math.*`, …) is portable. `cmd.exe` wrapping for `dir`/`cls` is Windows-only. Editor search is `nvim`/`vim`/`vi` on Unix, plus `.exe`/`notepad` on Windows.

Not implemented (and not planned as ash clones): job control (`jobs`, `fg`, `bg`, `&`), `trap`, `test`/`[`, `pushd`/`popd`, `getopts`, `ulimit`, `hash`. File tests use UFCS (`.exists()`, `.is_file()`, …) instead of `test -f`.

---

## 4. Environment variables

### 4.1 OS process environment

| Command | Effect |
|---|---|
| `export NAME=value` | `os.Setenv` **and** PCL scope `$NAME` |
| `export NAME value` | same |
| `export` / `env` | print `os.Environ()` |
| `unsetenv NAME...` | `os.Unsetenv` **and** unset PCL `$NAME` |
| `cd` | updates `PWD` and `OLDPWD` in both OS env and scope |

`$NAME` is **interpreter scope**, not a live `os.Getenv`. Host env is **not** auto-imported. To expose a host var: `export` it from the parent process, `export` it in `config.pcl`, or call FFI `os.Getenv`.

Path expansion (`shell.ExpandPath`) applies `~` and `os.ExpandEnv` (`$VAR` / `${VAR}`) to filesystem paths.

### 4.2 Config files

`config.pcl` (cwd), else `~/.pclrc`, else `--config path`, is a **normal PCL script** evaluated at startup after builtins are registered. `export`, `ai_config`, `ffi::bind`, and `load_go` are all valid there.

`ai_config` writes `ConfigService` keys only (`provider`, `api_key`, `model`, `prompt`, …). It does not set OS env. If `api_key` is `$ENVNAME`, the AI client reads `os.Getenv("ENVNAME")`.

### 4.3 Host variables PCL reads

| Variable | Purpose |
|---|---|
| `PCL_EDITOR` | preferred editor |
| `EDITOR` | fallback editor |
| `PCL_HISTORY` | history file path (default `~/.pcl_history`) |
| `PATH` | external command lookup |
| `OPENCODE_API_KEY` / `OPENAI_API_KEY` | AI client credentials |

---

## 5. Control-flow builtins

- `true` — returns boolean true; `$status` / `$?` = 0.
- `false` — returns boolean false; `$status` / `$?` = 1 (not a runtime error).
- `break` / `continue` — leave or skip the rest of the current `while` / `for` / `foreach` body. Outside a loop: runtime error.
- `if ([true]) ( ... )` works via command substitution; `if (true)` works as an expression token.

---

## 6. File builtins (`mv`, `cp`, `ln`)

- `mv [-f] src... dst` — rename; copy+remove if rename fails (cross-device). `-f` overwrites.
- `cp [-r] [-f] src... dst` — copy files; `-r` copies directories.
- `ln [-s] [-f] target link` — hard link (`os.Link`) or symlink (`-s`). Symlinks may fail on Windows without developer mode.

Multiple sources require `dst` to be a directory.

---

## 7. Grammar (implemented)

```ebnf
Program         ::= Statement* EOF
Statement       ::= Pipeline (";" | NEWLINE)*
Pipeline        ::= Command ( ("|" | "|>") ("$" Identifier)? Command )*
Command         ::= Assignment | SimpleCommand

Assignment      ::= (Identifier | SubscriptTarget) "=" Expression
SubscriptTarget ::= Identifier "[" (String | Integer | Expression) "]"

SimpleCommand   ::= Word+ Redirection*
Redirection     ::= "<" Word | ">" Word | ">>" Word | "2>" Word | "2>>" Word | "2>&1"

Word            ::= QuotedString | SingleQuotedString | CodeBlock | ArrayLiteral
                  | DictLiteral | CommandSubst | VariableRef | PerlQuote
                  | UFCSAccess | PlainWord

CodeBlock       ::= "(" Statement* ")"
ArrayLiteral    ::= "{" Word* "}"
DictLiteral     ::= "{" (Key ":" Value (","?))* "}"
CommandSubst    ::= "[" Statement "]"
VariableRef     ::= "$" Identifier | "${" Identifier "}"

PerlQuote       ::= "p"  Delimiter Text Delimiter ( "with" Options )?
                  | "p!" Delimiter Text Delimiter ( "with" Options )?
                  | "q"  Delimiter Text Delimiter
                  | "qq" Delimiter Text Delimiter
                  | "qx" Delimiter Text Delimiter
                  | "qw" Delimiter Words Delimiter

UFCSAccess      ::= Root ("." MethodCall | "." FieldAccess | "[" Subscript "]")*
Expression      ::= "(" LogicalOrExpr ")"
```

**Not in the grammar:** `&&` / `||` command lists, `&` background, `$(...)`, here-documents, `case`/`esac`. Use `if`/`while` and pipelines instead.

---

## 8. Prompting, tools, and editor (reference)

`p(...)` / `p!(...)` (stream) and `prompt` / `p` commands send a chat completion with registered tools. By default PCL **follows `tool_calls`**: execute, append `role=tool` observations, repeat until the model stops or `max_turns` (default 50). Opt out: `with { agent: false }` or `max_turns: 1`.

Reasoning is taken from API `reasoning_content` and `<think>…</think>` (`$x.reasoning`, `$x.thought`). Flattened tool calls from all turns stay on `$x.tools` / `$x.tools_used()`. Agent steps: `$x.steps`.

Default tools: `sh` (`cmd`), `read_file` (`path`), `write_file` (`path`, `content`), `read_skill` (`name`). User tools: `tool name (params) ( body )` — params are bound when the model calls the tool.

**Skills:** scan `~/.pcl/skills`, `<cwd>/.pcl/skills`, `<cwd>/.grok/skills`, `<cwd>/skills` for `*/SKILL.md`. Catalog (name + description) is appended to the system prompt. Full body is loaded only via `read_skill`. Rescan after `cd`; print gained/dropped names.

**MCP:** stdio only (`github.com/modelcontextprotocol/go-sdk`). `mcp add name -- cmd args…` spawns the server, `tools/list`, registers each tool as `name_toolname`. `mcp list` / `mcp tools` / `mcp remove`. Valid in `config.pcl`. Sessions closed on process exit. Not implemented: HTTP/SSE, OAuth, resources, prompts, sampling.

`$x.files()` scans tool-call paths and markdown fences. `.vim()` / `OpenMultipleInEditor` launches `PCL_EDITOR` or `EDITOR` or `nvim`/`vim` with `-p` for multiple files.

`ai_config` keys: `provider`, `api_base`, `api_key` (literal or `$ENV`), `model`, `temperature`, `system_prompt`, `prompt`, `multiline_prompt`.

FFI (`ffi::bind`, `load_go`) and pre-bound `math.*` / `strings.*` / `regexp.*` / `time.*` / `os.*` / `filepath.*` / `crypto.*` / `json.*` are embedding features, not required for shell use.

---

## 8.1 REPL inline streaming (`pkg/repl/repl.go`, `jobWriter`)

`p(...)` / `agent` jobs stream **inline** instead of into a pinned region:
completed lines are written above the prompt through readline's own stdout
writer (`wrapWriter` erases the prompt, emits, redraws it below), and the
in-flight partial line is rewritten in place (wrapped to the terminal width;
previous partial rows erased with `CSI A` + `CSI 2K` before each redraw).

Because there is no reserved screen region, there is no layout contract for
Ctrl+L, `clear`/`cls`, or terminal resizes to break — the terminal's native
scrollback and prompt behavior are untouched. All stream writes are
serialized under `REPL.outMu`.

Platform bits: `terminal_windows.go` / `terminal_unix.go` provide
`InitTerminal` (UTF-8 + VT mode on Windows), `TermWidth`, `TermHeight`.

Headless verification: `tests/conpty` spawns `pcl.exe` in a fresh console
(`CREATE_NEW_CONSOLE`), attaches with `AttachConsole`, injects keystrokes
with `WriteConsoleInput`, and scrapes the screen buffer
(`ReadConsoleOutputCharacterW`) at 80ms intervals into
`tests/conpty/frames.log`. (True ConPTY pipes do not deliver output inside
the sandbox, hence the attach-and-scrape approach.)

---

## 9. Language notes

**Blocks and arithmetic** — `( ... )` for bodies, conditions, and expressions:

```pcl
while ($counter <= 3) ( counter = ($counter + 1) )
if ($a > $b) ( puts "A > B" ) else ( puts "A <= B" )
```

**Arrays / dicts** — `{ apple banana }`, `{ name: "PCL", version: 1 }`.

**Staged pipelines** — `cat access.log |> $raw | grep "500" |> $errors`.

**UFCS files / regex** — `"f".exists()`, `.read()`, `.matches()`, `.replace()`.

**Agent / editor** — `p(...)` runs a tool-call loop by default (native `tool_calls`, execute, observe, repeat). Reasoning comes from `reasoning_content` and `<think>`. Opt out: `with { agent: false }`. `$x.files()`, `$x.vim()` (`nvim -p`).

## License

PolyForm Noncommercial License 1.0.0. Copyright (C) 2026 Xyzzy Apps \<xyzzyapps@gmail.com\>. Commercial use requires a separate license.

---

## 10. Verification

`go test -v ./tests/...` covers UTF-8 values, parser, interpreter (`while`, `break`/`continue`, procs), shell (`export` vs OS env, `true`/`false` `$status`, `mv`/`cp`/`ln`), pipelines, aliases, glob, history, FFI, and agent mocks.
