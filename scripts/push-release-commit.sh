#!/usr/bin/env bash
# Push an already committed release bump, rebasing over concurrent plugin
# release commits when needed. A release is never allowed to continue after a
# version bump failed to reach the repository.

set -euo pipefail

REMOTE="${1:-origin}"
BRANCH="${2:-main}"
MAX_ATTEMPTS="${3:-5}"
TRACKING_REF="refs/remotes/${REMOTE}/${BRANCH}"

for attempt in $(seq 1 "$MAX_ATTEMPTS"); do
  echo "Fetching ${REMOTE}/${BRANCH} before release push (attempt ${attempt}/${MAX_ATTEMPTS})..."
  if ! git fetch "$REMOTE" "+refs/heads/${BRANCH}:${TRACKING_REF}"; then
    echo "Could not fetch ${REMOTE}/${BRANCH}." >&2
    if [ "$attempt" -eq "$MAX_ATTEMPTS" ]; then
      exit 1
    fi
    sleep "$attempt"
    continue
  fi

  if ! git rebase "$TRACKING_REF"; then
    git rebase --abort || true
    echo "Release commit could not be rebased onto ${REMOTE}/${BRANCH}; refusing to publish a stale version bump." >&2
    exit 1
  fi

  if git push "$REMOTE" "HEAD:refs/heads/${BRANCH}"; then
    echo "Release commit pushed to ${REMOTE}/${BRANCH}."
    exit 0
  fi

  echo "${REMOTE}/${BRANCH} moved during the push; retrying after a fresh fetch."
  if [ "$attempt" -lt "$MAX_ATTEMPTS" ]; then
    sleep "$attempt"
  fi
done

echo "Could not push the release commit after ${MAX_ATTEMPTS} attempts." >&2
exit 1
