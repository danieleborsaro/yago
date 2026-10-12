#!/usr/bin/env bash
# Prints the Go packages affected by the changes between a base ref and HEAD,
# one import path per line. Prints "ALL" when the whole suite must run and
# nothing when no Go-relevant file changed.
#
# Usage: affected-packages.sh [base-ref]   (default: origin/main)
set -euo pipefail

BASE="${1:-origin/main}"

# Changes to these paths can affect any package.
FULL_RUN_REGEX='^(go\.mod|go\.sum|\.github/workflows/|tests/assets/|schemas/|schema-config\.json|tests/scripts/|internal/utils/)'

changed="$(git diff --name-only "$BASE"...HEAD)"

if [ -z "$changed" ]; then
  exit 0
fi

if grep -Eq "$FULL_RUN_REGEX" <<<"$changed"; then
  echo ALL
  exit 0
fi

module="$(go list -m)"

# Directories holding changed Go sources or test data.
dirs="$(grep -E '\.go$|/testdata/' <<<"$changed" | sed -E 's#/testdata/.*##; s#/[^/]*$##' | sort -u || true)"
if [ -z "$dirs" ]; then
  exit 0
fi

changed_pkgs=""
for d in $dirs; do
  [ -d "$d" ] && changed_pkgs+="$module/$d"$'\n'
done
[ -n "$changed_pkgs" ] || exit 0

# Build "package: dependencies" lines (test dependencies included) and select
# every package that depends on, or is, a changed package.
go list -test -f '{{.ImportPath}} {{join .Deps " "}}' ./... |
  awk -v changed="$changed_pkgs" '
    BEGIN {
      n = split(changed, c, "\n")
      for (i = 1; i <= n; i++) if (c[i] != "") want[c[i]] = 1
    }
    {
      pkg = $1
      sub(/\.test$/, "", pkg)
      sub(/_test$/, "", pkg)
      if (pkg in want) { hit[pkg] = 1; next }
      for (i = 2; i <= NF; i++) if ($i in want) { hit[pkg] = 1; break }
    }
    END { for (p in hit) print p }
  ' |
  sed -E 's/ \[.*\]$//' | sort -u |
  while read -r p; do
    go list -f '{{if or .TestGoFiles .XTestGoFiles}}{{.ImportPath}}{{end}}' "$p" 2>/dev/null || true
  done
