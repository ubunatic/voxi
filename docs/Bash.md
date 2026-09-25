<!-- harnez:variant=lite -->
# Bash Rules (Lite)

## 1. Header

```bash
#!/usr/bin/env bash
set -euo pipefail   # -e exit on non-zero; -u exit on unset var; -o pipefail last non-zero wins
```

## 2. Sourcing

DO `source f` — readable, greppable (`grep 'source '`), distinct from `./script.sh`.
DON'T `. f` — dot is lost in whitespace, confusable with paths, ungreppable.

```bash
source ~/.bashrc   # ✅
. ~/.bashrc        # ❌
```

## 3. Conditionals — TOP RULE: `if test`, never brackets

```bash
if test -f "$file"                    # ✅ 3-line if-then-fi
then printf 'Found %s\n' "$file"
fi

if test "$a" = "$b"                   # ✅ 4-line if-then-else-fi
then printf 'Equal\n'
else printf 'Not equal\n'
fi

if test -d "$dir"                     # ✅ multi-cmd: 1st cmd on `then` line, rest aligned under it
then printf 'Entering %s\n' "$dir"
     process_dir "$dir"
fi

while test "$x" != "$y"               # ✅ 1st cmd on `do` line
do process "$x"
done
```

DON'T (all forbidden, no exceptions, forget legacy usage):

- `[ x = y ]` single brackets
- `[[ x == y ]]` double brackets
- `; then` / `; do` — break the line instead
- `then` alone on its line (dangling) — 1st cmd goes on the `then` line
- `test x = y && do_a || do_b` logic ops as control flow – use 4-line `if-then-else-fi`

## 4. Variables

```bash
"$var" "${var}"                            # quote EVERY expansion
pattern="${1:?Usage: script.sh PATTERN}"   # required arg
f() {
   local name="$1"        # literal -> assign inline
   local result           # cmd substitution -> declare FIRST,
   result=$(cmd args)     # then assign; else `local` masks cmd exit code
}
```

## 5. Output

```bash
printf '%s\n' "$var"              # printf > echo for variables
printf 'ERROR: %s\n' "$msg" >&2   # errors -> stderr

pass() {
   printf '  ✓ %s\n' "$*"
}

fail() {
   printf 'ERROR: %s\n' "$*" >&2
   exit 1
}
```

## 6. Continuation — align, never fixed indent

```bash
result=$(some_command |        # pipeline: break after `|`, align under chain start
         grep "pattern" |
         awk '{print $2}')

if test -f "$a" &&             # condition: break after `&&`/`||`, align under 1st test
   test -d "$b"
then stmt1
     stmt2
else fail "not found"
fi

for item in "${array[@]}"      # then/else/do: 1st cmd on keyword line, rest aligned
do process_item "$item" || fail "err"
   log_item "$item"
done
```

Function body: base indent 3 inside `{ }`. Alignment continuation > fixed indent for conditionals.

## 7. Commands & traps

```bash
command -v tool   # not `which`
out=$(cmd 2>&1)   # capture
cmd > f           # write
cmd >> f          # append
cmd 2>/dev/null   # drop errors
tmp=$(mktemp); trap 'rm -f "$tmp"' EXIT   # always trap-clean temps

timeout 30 curl -sf https://example.com/health
# wrap any uncertain-duration cmd lacking its own client-side timeout (net probe,
# lock wait, external call). Size seconds per command, not one global value.
# timeout exits 124 on kill -> branch on it separately from the cmd's own codes.
# Per-invocation only; background jobs -> AgenticLoop.md "Blocking sleep Waits" /
# "Buffered Long-Running Output".
```

## 8. Functions

Define before first use. Status via `return 0`/`return 1` — never printed booleans.

## 9. Directory scoping — `-C` over `cd`

Rule: command has a directory flag -> use it; never `cd` just for scoping. cwd persists
across tool calls, so a stray `cd` silently retargets the *next* unrelated call (e.g.
`git status` on the wrong repo) with no error.

```bash
git -C DIR status                  # git    -C DIR
make -C DIR test                   # make   -C DIR
go -C DIR build ./...              # go     -C DIR (1.20+)
npm --prefix DIR install           # npm    --prefix DIR
cargo build --manifest-path DIR/Cargo.toml   # cargo --manifest-path FILE

(cd DIR && some-tool --flag)   # ✅ no flag exists -> subshell, cwd auto-restored
cd DIR && some-tool --flag     # ⚠️ leaks cwd through rest of THIS call only; keep in ONE call
                               # ❌ bare `cd` meant to carry into a LATER call
```

Shared-shell caveat: where the shell is shared with the user's interactive terminal, an
agent's `cd` outlives the call and moves the human's prompt too. Advisory, not enforceable
(issue 095) — the shell reaches anywhere the OS user can. Prefer `-C`/absolute paths/subshell;
if a bare `cd` is unavoidable (tool takes relative paths only), capture and restore:

```bash
orig=$(pwd)
cd DIR
some-tool --relative-only-flag
cd "$orig"
```

See issue 095 (cwd leak, trust boundary), issue 222 (multi-repo wrong-repo failure).

## 10. Awk Portability (rarely needed)

Default is mawk, not gawk — no 3-arg `match(str, /re/, arr)` (use `split()`/`sub()`/`gsub()`),
no `strtonum()` (write a manual `h2d()`), no `gensub()` (use `sub()`/`gsub()` + a temp var).

<!-- harnez:stop -->
