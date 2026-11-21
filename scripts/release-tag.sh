#!/usr/bin/env bash
# Prints the v* tag to cut for CHANGELOG.md's top "## x.y.z" heading, or nothing (#20).
# Nothing when the heading is "(Unreleased)" or its tag exists; fails if it would go backwards.
set -euo pipefail

changelog="${1:-CHANGELOG.md}"
heading="$(grep -m1 '^## ' "$changelog" || true)"

if [[ ! "$heading" =~ ^##\ ([0-9]+\.[0-9]+\.[0-9]+)[[:space:]]*$ ]]; then
  echo "release-tag: top heading '${heading}' is not a release; no tag" >&2
  exit 0
fi
tag="v${BASH_REMATCH[1]}"

if git rev-parse -q --verify "refs/tags/${tag}" >/dev/null; then
  echo "release-tag: ${tag} already exists; no tag" >&2
  exit 0
fi

latest="$(git tag -l 'v*' --sort=-v:refname | head -1)"
if [[ -n "$latest" && "$(printf '%s\n%s\n' "$latest" "$tag" | sort -V | tail -1)" != "$tag" ]]; then
  echo "release-tag: ${tag} is older than the latest tag ${latest}" >&2
  exit 1
fi

echo "$tag"
