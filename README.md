# bead (Go)

Human entry wizard for beads federation stores. Go rewrite of the original
bash wizard (`~/.local/bin/bead.bash-20260908`).

## Why

`<store> create "…"` puts everything in the TITLE and on the shell command
line, so long / multiline / quote-heavy content breaks on shell quoting,
length guards, or approval hooks. This wizard takes the entry in-app and
sends the long content through `--body-file` (the description), never over
the command line.

## Usage

```
bead                            pick a store, then entry form → create
bead <store>                    entry form for that store → create
bead <store> "Short title"      entry form with the title prefilled
bead "Short title"              pick a store with the title prefilled
echo "body" | bead <store> "T"  non-interactive: title + piped body
bead pi ["thought..."]          bead-scoped pi session: paste, scope down, create, exit
bead -n|--dry-run <store> …     preview; create nothing
bead --list                     list known stores and exit
bead -h|--help                  help
```

Flags (`-n`, `--dry-run`) accepted in any position.

## Picker

Bare `bead` opens a fullscreen picker over all 38 stores from
`~/.config/pai/stores.yaml` (bubbletea, alt-screen):

- Type any letters to filter (name + description); first match highlights.
- `↑↓` / `PgUp/PgDn` move; the 12-row viewport scrolls (`▲/▼ N more`).
- `⏎` selects, `esc` / `ctrl-c` bails. `backspace`, `ctrl-u`/`ctrl-w` edit.
- Deliberately no vim `j/k` navigation — those are filter input.

Unknown single args (e.g. `bead "Fix login"`) drop into the picker with
the arg as prefilled title instead of dying on "no such store command".
When stdout isn't a terminal the picker falls back to a plain name prompt
(`--list` always uses the static name + description style).

## Entry form

`bead <store>` — or picking a store — lands in the entry form. Focus starts
on Title; `tab` cycles Title → Priority → Labels → Description → Title,
`↑↓` move between fields the same way.

- **Title**: one line. Required (`ctrl+s` with it empty stays put).
- **Priority**: `←/→` steps 0–4 (clamped), or just type the digit.
- **Labels**: fuzzy search over the store's existing labels
  (fetched in background via `<store> label list-all` — the form opens
  instantly and suggestions pop in when ready; repeats within 5 minutes
  serve from `~/Library/Caches/bead/` instantly). `⏎` adds the highlighted match — or your typed
  text as a brand-new label. Empty-input `backspace` pops the last chip;
  any leftover typed text is included on submit (comma-separated).
- **Description**: multiline area, `⏎` for newlines. (`↑↓` still move
  fields, even here — by design.)
- `ctrl+s` creates (`-t task -p N -l … --body-file …`), `esc` backs out to
  the picker (or quits for direct `bead <store>`). A failed create reopens
  the form with everything preserved and the error shown.

## `bead pi` — bead-scoped pi session

`bead pi ["thought..."]` launches pi from `~/data` with a system prompt
carrying the live store list plus the creation workflow: paste messy
content, scope it down, pi creates via `<store> create --body-file`, prints
`CREATED: <ids>`. `echo body | bead pi ["title"]` folds stdin into the
initial prompt; `bead -n pi [...]` dry-runs via `pi -p` (plans, no writes).
`~/.local/bin/bead-pi` is a one-line shim to `bead pi`.

- `--narrow`: stripped-down pi — `--no-skills --no-context-files
  --tools bash,read`. The scope prompt embeds the FULL runbook itself
  (federation model, store routing, `--body-file` pattern, priority 0–4,
  `-t` type options, mandatory duplicate-check) because the global
  AGENTS.md never loads. Full mode stays additive instead: AGENTS.md
  already teaches the federation model, so the prompt only adds the live
  store list. Narrow keeps `-n` dry-run and `~/data` cwd behavior.
- `--theme <name>`: forwarded as pi `--use-theme`, default
  `rose-pine-moon` so bead sessions look distinct from stock pi.
  Available: pi built-ins `dark` / `light`, plus any
  `~/.pi/agent/themes/*.json` by file name (e.g. `rose-pine-moon`).
  A missing theme falls back to pi's dark default.

## Themes

`BEAD_THEME` selects the palette (default `rose-pine`):

- `rose-pine` — Rosé Pine main (Pine accents, Gold names, Rose hints)
- `moon` / `rose-pine-moon` — Rosé Pine Moon (cooler accents)
- `nord` — Nord (Frost accents, Yellow names, Red hints)

Unknown values fall back to Rosé Pine. Colors auto-disable when piped
and honor `NO_COLOR=1` / `TERM=dumb`.

## Notes

- TUI via bubbletea/bubbles; everything else stdlib. Static binary.
- TTY detection uses ioctl (matches bash `[ -t 0 ]`); `/dev/null` and pipes
  correctly count as non-tty.
- When stdout isn't a tty the fullscreen UI can't run: falls back to the
  legacy editor form (`$BEAD_EDITOR`, `$VISUAL`, `$EDITOR`, nano/vim/vi).
- Known quirk (pre-existing, in `bd` itself): under a dumb pty that doesn't
  answer OSC terminal queries, `bd create` can stall waiting for a terminal
  response. Real terminals are unaffected.

## Build / install

```
cd /Users/trilliumsmith/code/bead
go build -o ~/.local/bin/bead .
```
