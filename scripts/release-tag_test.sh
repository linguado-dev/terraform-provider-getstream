#!/usr/bin/env bash
# Tests for release-tag.sh against a scratch git repo (#20). Run: make test-release-tag
set -euo pipefail

script="$(cd "$(dirname "$0")" && pwd)/release-tag.sh"
repo="$(mktemp -d)"
trap 'rm -rf "$repo"' EXIT
cd "$repo"
git init -q && git config user.email t@example.com && git config user.name t
git commit -q --allow-empty -m init && git tag v0.3.1

fail=0
check() { # name, changelog top heading, want stdout, want exit
  printf '%s\n\nnotes\n\n## 0.1.0\n' "$2" > CHANGELOG.md
  set +e; got="$("$script" CHANGELOG.md 2>/dev/null)"; code=$?; set -e
  if [[ "$got" != "$3" || "$code" != "$4" ]]; then
    echo "FAIL $1: got '$got' exit $code, want '$3' exit $4"; fail=1
  else
    echo "ok   $1"
  fi
}

check "new patch release"      "## 0.3.2"              "v0.3.2" 0
check "new minor release"      "## 0.4.0"              "v0.4.0" 0
check "existing tag"           "## 0.3.1"              ""       0
check "unreleased heading"     "## 0.4.0 (Unreleased)" ""       0
check "non-version heading"    "## Next"               ""       0
check "backwards version"      "## 0.2.9"              ""       1
check "trailing space allowed" "## 0.3.2 "             "v0.3.2" 0

exit $fail
