# bead (Go)

Human entry wizard for beads federation stores. Go rewrite of the original
bash wizard (`~/.local/bin/bead.bash-20260908`).

## Why

`<store> create "…"` puts everything in the TITLE and on the shell command
line, so long / multiline / quote-heavy content breaks on shell quoting,
length guards, or approval hooks. This wizard takes the entry in-app and
sends the long content through `--body-file -` (the description on stdin,
so it also works through brain remote wrappers), never over the command
line.

## Usage

```
bead                            pick a store, then entry form → create
bead <store>                    entry form for that store → create
bead <store> "Short title"      entry form with the title prefilled
bead "Short title"              pick a store with the title prefilled
echo "body" | bead <store> "T"  non-interactive: title + piped body
bead pi ["thought..."]          bead-scoped pi session: paste, scope down, create, exit
bead decide [store]             answer the decisions waiting on you, one at a time
bead decide --list [store]      plain list of those decisions
bead -n|--dry-run <store> …     preview; create nothing
bead --list                     list known stores and exit
bead -h|--help                  help
```

Flags (`-n`, `--dry-run`) accepted in any position.

## Picker

Bare `bead` opens a fullscreen picker over all 38 stores from
`~/.config/trillium-stores/stores.yaml` (on a brain remote client: the host's
`brain stores list --json`; bubbletea, alt-screen):

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
on Title; `tab` cycles Title → Priority → Labels → Description → Save → Title,
`↑↓` move between fields the same way.

- **Title**: one line. Required (creating with it empty stays put).
- **Priority**: `←/→` steps 0–4 (clamped), or just type the digit.
- **Labels**: fuzzy search over the store's existing labels
  (fetched in background via `<store> label list-all` — the form opens
  instantly and suggestions pop in when ready; repeats within 5 minutes
  serve from `~/Library/Caches/bead/` instantly). `⏎` adds the highlighted match — or your typed
  text as a brand-new label. Empty-input `backspace` pops the last chip;
  any leftover typed text is included on submit (comma-separated).
- **Description**: multiline area, `⏎` for newlines. (`↑↓` still move
  fields, even here — by design.)
- **Save**: a button after Description; `⏎` on it creates, same as `ctrl+s`.
- `ctrl+s` creates from any field (`-t task -p N -l … --body-file -`),
  `esc` backs out to the picker (or quits for direct `bead <store>`). A
  failed create reopens the form with everything preserved and the error
  shown.

## `bead pi` — bead-scoped pi session

`bead pi ["thought..."]` launches pi from `~/data` with a system prompt
carrying the live store list plus the creation workflow: paste messy
content, scope it down, pi creates via `<store> create --body-file -`
(body heredoc on stdin, so remote store wrappers work too), prints
`CREATED: <ids>`. `echo body | bead pi ["title"]` folds stdin into the
initial prompt; `bead -n pi [...]` dry-runs via `pi -p` (plans, no writes).
`~/.local/bin/bead-pi` is a one-line shim to `bead pi`.

- `--narrow`: stripped-down pi — `--no-skills --no-context-files
  --tools bash,read`. The scope prompt embeds the FULL runbook itself
  (federation model, store routing, stdin `--body-file -` pattern,
  priority 0–4, `-t` type options, mandatory duplicate-check) because the
  global AGENTS.md never loads. Full mode stays additive instead: AGENTS.md
  already teaches the federation model, so the prompt only adds the live
  store list. Narrow keeps `-n` dry-run and `~/data` cwd behavior.
- `--theme <name>`: forwarded as pi `--use-theme`, default
  `rose-pine-moon` so bead sessions look distinct from stock pi.
  Available: pi built-ins `dark` / `light`, plus any
  `~/.pi/agent/themes/*.json` by file name (e.g. `rose-pine-moon`).
  A missing theme falls back to pi's dark default.

## `bead decide` — answer the decisions waiting on you

`bead decide [store]` works through the open items labelled `captain-decision`
— in one store, or in every brain store on PATH when none is given — and saves
each answer as a comment on the item. It is the entry form's counterpart for
reading: the same keys and look, aimed at `task show` / `task comments add`.

- **List**: highest priority first (P0 on top), newest first within a priority.
  `↑↓` move, `⏎` opens an item, `esc` quits. Items already marked
  `decision-answered` are left out (`--all` shows them with a ✓).
- **Item**: the full text — title, description (its headings picked out),
  links, and the existing comments — in a scrollable pane (`pgup/pgdn` or
  `alt+↑/↓`). Below it, the same field cycle as the entry form: `tab`/`↑↓`
  between **Pick**, **Answer**, **After saving** and **Save answer**.
- **Pick**: shown when the item lists options under an `OPTIONS` heading
  (bullets, `A)` letters or `1.` numbers; `(recommended)` gets a ★). Press an
  option's number or letter, or `←/→` then `⏎`, to fill the answer with it; add
  a note on top if you like. Items without options just take typed text.
- **Answer**: required. Saved as `Answer (captain, <date>): …`, so firstmate can
  find it in the thread. The text goes to the store on stdin
  (`comments add <id> -f /dev/stdin`), so it works through remote store
  wrappers too.
- **After saving** (`←/→`): `mark answered` (default: adds the
  `decision-answered` label so the item leaves your queue while staying open for
  firstmate to act on and close), `comment only`, or `close the item` — closing
  only ever happens if you pick it, and never without an answer.
- `ctrl+s` (or `⏎` on Save) saves and moves on to the next unanswered item;
  `ctrl+n` / `ctrl+p` skip without answering; `esc` goes back to the list.
- `bead decide --list [store]` prints the same queue plainly (also what you
  get when stdout isn't a terminal).
- `-n` / `--dry-run` walks the same screens but runs no writes, printing the
  commands it would have run when you leave.

A store that can't be read is a warning, not a failure.

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
