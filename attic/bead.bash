#!/usr/bin/env bash
# bead — human entry wizard for beads federation stores.
#
# The problem it solves: `<store> create "…"` puts everything in the TITLE and on
# the shell command line, so long / multiline / quote-heavy content breaks on shell
# quoting, length guards, or approval hooks. This wizard opens your editor with a
# small form and sends the long content through `--body-file` (the description),
# never over the command line.
#
# Usage:
#   bead <store>                 open an editor form, then create the bead
#   bead <store> "Short title"   prefill the title, still edit the body
#   echo "body" | bead <store> "Short title"   non-interactive: title + piped body
#   bead --dry-run <store> …     show the create command / preview; create nothing
#
# Flags:
#   -n, --dry-run   forward --dry-run to the store's create (previews, no write);
#                   the editor draft is kept so you can rerun for real.
#
# Env:
#   BEAD_EDITOR   override editor (default: $VISUAL, then $EDITOR, then nano/vim/vi)
#
# Works with any federation store whose CLI wrapper is on PATH (talon, task, brain, …).

set -o pipefail

SENTINEL='>>>>> DESCRIPTION BELOW — keep this line; everything under it is the body >>>>>'

die() { printf 'bead: %s\n' "$1" >&2; exit "${2:-1}"; }

# ---- collect flags (any position) before the positional store/title ---------
dry_run=0
args=()
for a in "$@"; do
  case "$a" in
    -n|--dry-run) dry_run=1 ;;
    *) args+=("$a") ;;
  esac
done
set -- "${args[@]}"

store="${1:-}"
if [ -z "$store" ]; then
  {
    echo "usage: bead <store> [\"title\"]"
    echo "stores:"
    reg="$HOME/.config/pai/stores.yaml"
    [ -f "$reg" ] && sed -n 's/^    \([a-z_][a-z0-9_-]*\):$/  \1/p' "$reg"
  } >&2
  exit 2
fi
shift
prefill_title="${1:-}"

command -v "$store" >/dev/null 2>&1 || die "no such store command: $store (is its wrapper on PATH?)" 2

# ---- non-interactive: body piped on stdin ----------------------------------
if [ ! -t 0 ]; then
  [ -n "$prefill_title" ] || die "a title is required:  bead $store \"Title\" < body" 2
  bodyfile="$(mktemp -t bead-body.XXXXXX)"
  cat > "$bodyfile"
  trap 'rm -f "$bodyfile"' EXIT
  set -- "$store" create "$prefill_title"
  [ -s "$bodyfile" ] && set -- "$@" --body-file "$bodyfile"
  [ "$dry_run" -eq 1 ] && set -- "$@" --dry-run
  exec "$@"
fi

# ---- interactive: editor form ----------------------------------------------
tmpl="$(mktemp -t bead-form.XXXXXX.md)"
{
  echo "Title: $prefill_title"
  echo "Type: task"
  echo "Priority: 2"
  echo "Labels:"
  echo "# Type: bug|feature|task|epic|chore|decision    Priority: 0-4 (0=highest)"
  echo "# Lines starting with # are ignored. Keep the Title short (one line)."
  echo "$SENTINEL"
  echo ""
} > "$tmpl"

ed="${BEAD_EDITOR:-${VISUAL:-${EDITOR:-}}}"
if [ -z "$ed" ]; then
  for c in nano vim vi; do command -v "$c" >/dev/null 2>&1 && { ed="$c"; break; }; done
fi
[ -n "$ed" ] || die "no editor found; set \$EDITOR or \$BEAD_EDITOR" 2

# shellcheck disable=SC2086  # intentional word-split so 'code -g --wait' works
$ed "$tmpl" || die "editor exited non-zero; draft kept at $tmpl"

header="$(awk -v s="$SENTINEL" '$0==s{exit} {print}' "$tmpl")"
field() { printf '%s\n' "$header" | grep -m1 "^$1:" | sed "s/^$1:[[:space:]]*//" | tr -d '\r'; }
title="$(field Title)"
type="$(field Type)";     type="${type:-task}"
prio="$(field Priority)"; prio="${prio:-2}"
labels="$(field Labels)"

body="$(awk -v s="$SENTINEL" 'f{print} $0==s{f=1}' "$tmpl" | sed '/./,$!d')"   # drop leading blank lines
[ -z "$(printf '%s' "$body" | tr -d '[:space:]')" ] && body=""                  # all-whitespace -> empty

[ -n "$title" ] || die "empty Title — aborted. Your draft is kept at: $tmpl"

set -- "$store" create "$title" -t "$type" -p "$prio"
[ -n "$labels" ] && set -- "$@" -l "$labels"
if [ -n "$body" ]; then
  bodyfile="$(mktemp -t bead-body.XXXXXX)"
  printf '%s\n' "$body" > "$bodyfile"
  set -- "$@" --body-file "$bodyfile"
fi
[ "$dry_run" -eq 1 ] && set -- "$@" --dry-run

"$@"; rc=$?
[ -n "${bodyfile:-}" ] && rm -f "$bodyfile"
if [ "$dry_run" -eq 1 ]; then
  printf 'bead: dry run — nothing created; draft kept at %s\n' "$tmpl" >&2
elif [ "$rc" -eq 0 ]; then
  rm -f "$tmpl"
else
  printf 'bead: create failed (rc=%s); draft kept at %s\n' "$rc" "$tmpl" >&2
fi
exit "$rc"
