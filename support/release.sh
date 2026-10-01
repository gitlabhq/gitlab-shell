#!/usr/bin/env bash
#
# Automates the gitlab-shell release process. See "Release Process" in README.md.
#
# Usage:
#   support/release.sh changelog   Print the CHANGELOG entries since the latest release tag.
#   support/release.sh prepare     Open a "Release vX.Y.Z" MR that bumps VERSION and CHANGELOG.
#   support/release.sh tag         Tag the current commit with the version from VERSION.
#
# Environment:
#   RELEASE_BUMP                  patch (default), minor or major. Used by `prepare`.
#   GITLAB_SHELL_RELEASE_TOKEN    Project access token with the `api` scope. CI_JOB_TOKEN
#                                 can't be used: it can't create commits or merge requests,
#                                 and tags created with it don't trigger tag pipelines.
#   DRY_RUN                       Set to 1 to print what would happen without calling the API.
#
# The CI_* variables are provided by GitLab CI. Defaults allow running the script locally.

set -euo pipefail

CI_API_V4_URL="${CI_API_V4_URL:-https://gitlab.com/api/v4}"
CI_PROJECT_ID="${CI_PROJECT_ID:-14022}"
CI_DEFAULT_BRANCH="${CI_DEFAULT_BRANCH:-main}"
CI_COMMIT_SHA="${CI_COMMIT_SHA:-$(git rev-parse HEAD)}"
DRY_RUN="${DRY_RUN:-0}"

RELEASE_TAG_PATTERN='^v[0-9]+\.[0-9]+\.[0-9]+$'
RELEASE_LABELS="type::maintenance,maintenance::release,group::source code,devops::create,section::dev,Category:Source Code Management"

log() {
  echo "$@" >&2
}

fail() {
  log "ERROR: $*"
  exit 1
}

dry_run() {
  [[ "$DRY_RUN" == "1" ]]
}

## Calls the GitLab API for the current project.
## Arguments: HTTP method, path relative to the project, extra curl arguments.
api() {
  local method="$1" path="$2"
  shift 2

  : "${GITLAB_SHELL_RELEASE_TOKEN:?must be set to a project access token with the api scope}"

  curl --silent --show-error --fail-with-body \
    --request "$method" \
    --header "PRIVATE-TOKEN: ${GITLAB_SHELL_RELEASE_TOKEN}" \
    --header "Content-Type: application/json" \
    "${CI_API_V4_URL}/projects/${CI_PROJECT_ID}/${path}" \
    "$@"
}

urlencode() {
  jq -rn --arg value "$1" '$value | @uri'
}

## Prints the highest vX.Y.Z tag.
latest_release_tag() {
  git tag --list 'v*' --sort=-v:refname | grep -E "$RELEASE_TAG_PATTERN" | head -n1 || true
}

tag_exists() {
  git rev-parse --quiet --verify "refs/tags/$1" >/dev/null
}

## Prints the version from the VERSION file at the given revision.
version_at() {
  git show "$1:VERSION" | tr -d '[:space:]'
}

## Prints the CHANGELOG entries for the merge requests merged in the range.
##
## Every merge to the default branch creates a merge commit whose message
## contains the merge request title on the third line and a
## "See merge request <reference>" line, so the first-parent merge commits are
## exactly the list of merged merge requests, newest first.
changelog_entries() {
  local range="$1" sha message title iid

  for sha in $(git log --first-parent --merges --format='%H' "$range"); do
    message="$(git log -1 --format='%B' "$sha")"
    title="$(sed -n '3p' <<<"$message")"
    iid="$(sed -nE 's/^See merge request .*(!|merge_requests\/)([0-9]+).*$/\2/p' <<<"$message" | tail -n1)"

    # Previous release merge requests aren't changes.
    if [[ -z "$title" || "$title" =~ ^Release\ v[0-9] ]]; then
      continue
    fi

    if [[ -n "$iid" ]]; then
      echo "- ${title} !${iid}"
    else
      echo "- ${title}"
    fi
  done
}

## Prints the next version. Arguments: current version, bump type.
bump_version() {
  local major minor patch
  IFS='.' read -r major minor patch <<<"$1"

  case "$2" in
    major) echo "$((major + 1)).0.0" ;;
    minor) echo "${major}.$((minor + 1)).0" ;;
    patch) echo "${major}.${minor}.$((patch + 1))" ;;
    *) fail "RELEASE_BUMP must be one of patch, minor or major, got '$2'" ;;
  esac
}

## Fails unless the latest release tag matches VERSION at the given revision.
## Otherwise a release merge request was merged without being tagged, and a new
## release would skip that version.
ensure_last_release_tagged() {
  local revision="$1" latest_tag="$2" version
  version="$(version_at "$revision")"

  if [[ "v${version}" != "$latest_tag" ]]; then
    fail "VERSION is ${version}, but the latest release tag is ${latest_tag}. Tag v${version} first (support/release.sh tag)."
  fi
}

cmd_changelog() {
  git fetch --tags --quiet origin || log "Could not fetch tags, using local tags"

  local latest_tag
  latest_tag="$(latest_release_tag)"
  [[ -n "$latest_tag" ]] || fail "No vX.Y.Z tag found"

  log "Changes since ${latest_tag}:"
  changelog_entries "${latest_tag}..${CI_COMMIT_SHA}"
}

cmd_prepare() {
  local bump="${RELEASE_BUMP:-patch}"

  git fetch --tags --quiet origin

  local latest_tag
  latest_tag="$(latest_release_tag)"
  [[ -n "$latest_tag" ]] || fail "No vX.Y.Z tag found"
  log "Latest release tag: ${latest_tag}"

  ensure_last_release_tagged "$CI_COMMIT_SHA" "$latest_tag"

  local entries
  entries="$(changelog_entries "${latest_tag}..${CI_COMMIT_SHA}")"
  if [[ -z "$entries" ]]; then
    log "No merge requests merged since ${latest_tag}, nothing to release."
    return 0
  fi

  local version tag branch
  version="$(bump_version "${latest_tag#v}" "$bump")"
  tag="v${version}"
  branch="release-${version//./-}"

  tag_exists "$tag" && fail "Tag ${tag} already exists"

  log "Preparing ${tag} (${bump}) from ${CI_COMMIT_SHA} on branch ${branch}:"
  log "$entries"

  changelog_file="$(mktemp)"
  trap 'rm -f "$changelog_file"' EXIT
  {
    printf '%s\n\n%s\n\n' "$tag" "$entries"
    git show "${CI_COMMIT_SHA}:CHANGELOG"
  } >"$changelog_file"

  if dry_run; then
    log "DRY_RUN: would commit VERSION=${version} and this CHANGELOG to ${branch}:"
    head -n "$(($(wc -l <<<"$entries") + 3))" "$changelog_file"
    return 0
  fi

  local existing_mr
  existing_mr="$(api GET "merge_requests?state=opened&source_branch=$(urlencode "$branch")" | jq -r '.[0].web_url // empty')"
  if [[ -n "$existing_mr" ]]; then
    log "Release merge request already exists: ${existing_mr}"
    return 0
  fi

  jq -n \
    --arg branch "$branch" \
    --arg start_sha "$CI_COMMIT_SHA" \
    --arg message "Release ${tag}" \
    --arg version "${version}"$'\n' \
    --rawfile changelog "$changelog_file" \
    '{
      branch: $branch,
      start_sha: $start_sha,
      commit_message: $message,
      actions: [
        { action: "update", file_path: "VERSION", content: $version },
        { action: "update", file_path: "CHANGELOG", content: $changelog }
      ]
    }' | api POST "repository/commits" --data @- >/dev/null

  local mr_url
  mr_url="$(
    jq -n \
      --arg branch "$branch" \
      --arg target "$CI_DEFAULT_BRANCH" \
      --arg title "Release ${tag}" \
      --arg description "Prepares for the ${tag}"$'\n\n'"${entries}" \
      --arg labels "$RELEASE_LABELS" \
      --arg assignee "${GITLAB_USER_ID:-}" \
      '{
        source_branch: $branch,
        target_branch: $target,
        title: $title,
        description: $description,
        labels: $labels,
        remove_source_branch: true,
        squash: true
      } + (if $assignee == "" then {} else { assignee_id: ($assignee | tonumber) } end)' |
      api POST "merge_requests" --data @- | jq -r '.web_url'
  )"

  log "Created release merge request: ${mr_url}"
}

cmd_tag() {
  git fetch --tags --quiet origin

  local version tag
  version="$(version_at "$CI_COMMIT_SHA")"
  tag="v${version}"
  [[ "$tag" =~ $RELEASE_TAG_PATTERN ]] || fail "VERSION '${version}' is not a valid X.Y.Z version"

  # Only tag the commit that changed VERSION, so re-running the job on a later
  # commit can't tag unrelated changes.
  if git diff --quiet "${CI_COMMIT_SHA}^1" "$CI_COMMIT_SHA" -- VERSION; then
    log "VERSION didn't change in ${CI_COMMIT_SHA}, nothing to tag."
    return 0
  fi

  # Never move or recreate an existing tag: it's mirrored to the security and
  # dev repositories, and recreating it breaks the mirror sync.
  if tag_exists "$tag"; then
    log "Tag ${tag} already exists, nothing to do."
    return 0
  fi

  local latest_tag
  latest_tag="$(latest_release_tag)"
  if [[ -n "$latest_tag" && "$(printf '%s\n%s\n' "$latest_tag" "$tag" | sort -V | tail -n1)" != "$tag" ]]; then
    fail "${tag} is lower than the latest release tag ${latest_tag}"
  fi

  local changelog_head
  changelog_head="$(git show "${CI_COMMIT_SHA}:CHANGELOG" | head -n1 | tr -d '[:space:]')"
  [[ "$changelog_head" == "$tag" ]] || fail "CHANGELOG starts with '${changelog_head}', expected '${tag}'"

  if dry_run; then
    log "DRY_RUN: would create tag ${tag} at ${CI_COMMIT_SHA}"
    return 0
  fi

  jq -n --arg tag "$tag" --arg ref "$CI_COMMIT_SHA" \
    '{ tag_name: $tag, ref: $ref, message: ("Release " + $tag) }' |
    api POST "repository/tags" --data @- >/dev/null

  log "Created tag ${tag} at ${CI_COMMIT_SHA}"
}

main() {
  case "${1:-}" in
    changelog) cmd_changelog ;;
    prepare) cmd_prepare ;;
    tag) cmd_tag ;;
    *)
      sed -n '3,8p' "$0" | sed 's/^# \{0,1\}//' >&2
      exit 1
      ;;
  esac
}

main "$@"
