#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TESTDIR="$ROOT/_lua54-subset-test"
MANIFEST="$TESTDIR/manifest.tsv"

if [[ ! -f "$MANIFEST" ]]; then
  echo "manifest not found: $MANIFEST" >&2
  exit 1
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

TOLANG="$TMPDIR/tolang"
COMPILECHECK="$TMPDIR/lua54_compilecheck"

echo "[lua54-subset] building tools..."
(cd "$ROOT" && go build -o "$TOLANG" ./cmd/tolang)
(cd "$ROOT" && go build -o "$COMPILECHECK" ./_tools/lua54_compilecheck)

run_with_timeout() {
  if command -v timeout >/dev/null 2>&1; then
    timeout 8s "$@"
  else
    "$@"
  fi
}

total=0
pass=0
skip=0
fail=0

FAIL_LOG="$TMPDIR/failures.log"
: > "$FAIL_LOG"

while IFS=$'\t' read -r mode file reason; do
  if [[ -z "${mode:-}" || "$mode" == \#* ]]; then
    continue
  fi

  src="$TESTDIR/$file"
  if [[ ! -f "$src" ]]; then
    echo "missing test file: $file" >&2
    fail=$((fail+1))
    total=$((total+1))
    printf 'missing\t%s\t%s\n' "$file" "file not found" >> "$FAIL_LOG"
    continue
  fi

  if [[ "$mode" == "skip" ]]; then
    skip=$((skip+1))
    total=$((total+1))
    continue
  fi

  total=$((total+1))
  san="$TMPDIR/$file"
  mkdir -p "$(dirname "$san")"
  awk 'NR==1 && /^#/ {next} {print}' "$src" > "$san"

  case "$mode" in
    runtime)
      if run_with_timeout "$TOLANG" "$san" >/dev/null 2>&1; then
        pass=$((pass+1))
      else
        fail=$((fail+1))
        out=$(run_with_timeout "$TOLANG" "$san" 2>&1 | sed -n '1p' || true)
        printf 'runtime\t%s\t%s\n' "$file" "${out:-non-zero exit}" >> "$FAIL_LOG"
      fi
      ;;
    compile)
      if "$COMPILECHECK" "$san" >/dev/null 2>&1; then
        pass=$((pass+1))
      else
        fail=$((fail+1))
        out=$("$COMPILECHECK" "$san" 2>&1 | sed -n '1p' || true)
        printf 'compile\t%s\t%s\n' "$file" "${out:-non-zero exit}" >> "$FAIL_LOG"
      fi
      ;;
    *)
      fail=$((fail+1))
      printf 'manifest\t%s\t%s\n' "$file" "unknown mode '$mode'" >> "$FAIL_LOG"
      ;;
  esac

done < "$MANIFEST"

echo "[lua54-subset] summary: total=$total pass=$pass skip=$skip fail=$fail"
if [[ "$fail" -ne 0 ]]; then
  echo "[lua54-subset] failures:"
  sed -n '1,200p' "$FAIL_LOG"
  exit 1
fi
