#!/usr/bin/env bash
#
# Automates the gitlab-shell release process. See "Release Process" in README.md.
#
# Usage:
#   support/release.sh changelog   Print the CHANGELOG entries since the latest release tag.
#   support/release.sh prepare     Open a "Release vX.Y.Z" MR that bumps VERSION and CHANGELOG.
#   support/release.sh tag         Tag the current commit with the version from VERSION and
#                                  create the GitLab release with its CHANGELOG section.
#
# Environment:
#   RELEASE_BUMP                  patch (default), minor or major. Used by `prepare`.
#   GITLAB_SHELL_RELEASE_TOKEN    Project access token with the `api` scope, passed to glab.
#                                 CI_JOB_TOKEN can't be used: it can't create commits or
#                                 merge requests, and tags created with it don't trigger
#                                 tag pipelines.
#   SECURITY_REPO_READ_TOKEN      Token with the `read_api` scope and at least the Reporter
#                                 role in the security repository (SECURITY_PROJECT_PATH).
#                                 Used to refuse versions that a security release already
#                                 tagged there, which aren't visible from this repository.
#   DRY_RUN                       Set to 1 to print what would happen without calling the API.
#                                 The read-only security repository check still runs when
#                                 SECURITY_REPO_READ_TOKEN is set.
#
# Requires git, jq and glab (https://gitlab.com/gitlab-org/cli). The CI_* variables
# are provided by GitLab CI. Defaults allow running the script locally.

set -euo pipefail

CI_PROJECT_ID="${CI_PROJECT_ID:-14022}"
CI_PROJECT_PATH="${CI_PROJECT_PATH:-gitlab-org/gitlab-shell}"
CI_DEFAULT_BRANCH="${CI_DEFAULT_BRANCH:-main}"
CI_COMMIT_SHA="${CI_COMMIT_SHA:-$(git rev-parse HEAD)}"
SECURITY_PROJECT_PATH="${SECURITY_PROJECT_PATH:-gitlab-org/security/gitlab-shell}"
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

## Runs glab authenticated with the given token. The token is passed inline
## because a GITLAB_TOKEN CI/CD variable would take precedence over job variables.
glab_with_token() {
  local token="$1"
  shift

  GITLAB_TOKEN="$token" GITLAB_HOST="${CI_SERVER_HOST:-gitlab.com}" glab "$@"
}

## Runs glab authenticated with the release token.
glab_release() {
  : "${GITLAB_SHELL_RELEASE_TOKEN:?must be set to a project access token with the api scope}"

  glab_with_token "$GITLAB_SHELL_RELEASE_TOKEN" "$@"
}

## Prints the HTTP status of a GET request, or 000 if there was no response.
## Arguments: token, API path.
http_status() {
  local status
  status="$(glab_with_token "$1" api --include "$2" 2>/dev/null | head -n1 | awk '{ print $2 }' || true)"

  echo "${status:-000}"
}

## Fails if the tag already exists in the security repository.
##
## Security releases are tagged in the security repository first, and those tags
## stay invisible here until the release is synced back. Releasing the same
## version from this repository would make the repositories diverge and break
## the mirror sync, so fail closed: no release is better than a duplicate version.
ensure_tag_free_in_security_repo() {
  local tag="$1" project status
  project="${SECURITY_PROJECT_PATH//\//%2F}"

  if [[ -z "${SECURITY_REPO_READ_TOKEN:-}" ]]; then
    if dry_run; then
      log "DRY_RUN: SECURITY_REPO_READ_TOKEN isn't set, skipping the ${SECURITY_PROJECT_PATH} check"
      return 0
    fi

    fail "SECURITY_REPO_READ_TOKEN must be set to check ${tag} against ${SECURITY_PROJECT_PATH}." \
      "Use a read_api token with at least the Reporter role in that project."
  fi

  # GitLab returns 404 for a private project the token can't see, which would
  # read as "tag is free". Check access to the project first.
  status="$(http_status "$SECURITY_REPO_READ_TOKEN" "projects/${project}")"
  if [[ "$status" != "200" ]]; then
    fail "SECURITY_REPO_READ_TOKEN can't read ${SECURITY_PROJECT_PATH} (HTTP ${status})." \
      "The token is most likely expired, revoked, or no longer a member of that project."
  fi

  status="$(http_status "$SECURITY_REPO_READ_TOKEN" "projects/${project}/repository/tags/${tag}")"
  case "$status" in
    404)
      log "${tag} is free in ${SECURITY_PROJECT_PATH}"
      ;;
    200)
      fail "${tag} is already tagged in ${SECURITY_PROJECT_PATH} by a security release." \
        "Wait until the security release is synced back to ${CI_PROJECT_PATH}, or choose the next free version."
      ;;
    *)
      fail "Could not check ${tag} in ${SECURITY_PROJECT_PATH} (HTTP ${status})."
      ;;
  esac
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
## Every merge to the default branch creates a merge commit from the project's
## merge commit template (Settings > Merge requests), which starts with:
##
##   Merge branch '<source>' into '<target>'
##
##   <merge request title>
##   ...
##   See merge request <reference>
##
## so the first-parent merge commits are exactly the list of merged merge
## requests, newest first. A merge commit in any other format fails loudly
## instead of being dropped from the CHANGELOG, for example if the template
## changes.
changelog_entries() {
  local range="$1" sha message title iid

  for sha in $(git log --first-parent --merges --format='%H' "$range"); do
    message="$(git log -1 --format='%B' "$sha")"
    title="$(sed -n '3p' <<<"$message")"
    iid="$(sed -nE 's/^See merge request .*(!|merge_requests\/)([0-9]+).*$/\2/p' <<<"$message" | tail -n1)"

    if ! [[ "$(sed -n '1p' <<<"$message")" =~ ^Merge\ branch\ \'.+\'\ into\ \'.+\'$ &&
      -z "$(sed -n '2p' <<<"$message")" && -n "$title" && -n "$iid" ]]; then
      fail "Merge commit ${sha} doesn't match the expected merge commit template," \
        "so its merge request can't be added to the CHANGELOG. Check the project's merge commit template," \
        "or write the CHANGELOG entry by hand."
    fi

    # Previous release merge requests aren't changes.
    if [[ "$title" =~ ^Release\ v[0-9] ]]; then
      continue
    fi

    echo "- ${title} !${iid}"
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

  if tag_exists "$tag"; then
    fail "Tag ${tag} already exists, but it isn't the latest release tag on ${CI_DEFAULT_BRANCH}." \
      "It was most likely tagged by a security or backport release. Choose the next free version."
  fi
  ensure_tag_free_in_security_repo "$tag"

  log "Preparing ${tag} (${bump}) from ${CI_COMMIT_SHA} on branch ${branch}:"
  log "$entries"

  # Global so the EXIT trap can still remove it after this function returns.
  CHANGELOG_FILE="$(mktemp)"
  trap 'rm -f "$CHANGELOG_FILE"' EXIT
  {
    printf '%s\n\n%s\n\n' "$tag" "$entries"
    git show "${CI_COMMIT_SHA}:CHANGELOG"
  } >"$CHANGELOG_FILE"

  if dry_run; then
    log "DRY_RUN: would commit VERSION=${version} and this CHANGELOG to ${branch}:"
    head -n "$(($(wc -l <<<"$entries") + 3))" "$CHANGELOG_FILE"
    return 0
  fi

  local existing_mr
  existing_mr="$(glab_release mr list --repo "$CI_PROJECT_PATH" --source-branch "$branch" --output json --jq '.[0].web_url // empty')"
  if [[ -n "$existing_mr" ]]; then
    log "Release merge request already exists: ${existing_mr}"
    return 0
  fi

  # Once a branch exists, the commits API ignores start_sha and appends to the
  # branch, so a branch left by a failed run or a closed merge request would
  # put a stale commit in the release. Don't delete it automatically, it may
  # have someone's edits.
  local branch_status
  branch_status="$(http_status "$GITLAB_SHELL_RELEASE_TOKEN" "projects/${CI_PROJECT_ID}/repository/branches/${branch}")"
  case "$branch_status" in
    404) ;;
    200)
      fail "Branch ${branch} already exists without an open merge request." \
        "It was most likely left by a failed run or a closed merge request. Delete it and run the job again."
      ;;
    *)
      fail "Could not check whether branch ${branch} exists (HTTP ${branch_status})."
      ;;
  esac

  # A single commit with both files, created from the exact commit the
  # changelog was computed from. glab has no command for this, so use the API.
  jq -n \
    --arg branch "$branch" \
    --arg start_sha "$CI_COMMIT_SHA" \
    --arg message "Release ${tag}" \
    --arg version "${version}"$'\n' \
    --rawfile changelog "$CHANGELOG_FILE" \
    '{
      branch: $branch,
      start_sha: $start_sha,
      commit_message: $message,
      actions: [
        { action: "update", file_path: "VERSION", content: $version },
        { action: "update", file_path: "CHANGELOG", content: $changelog }
      ]
    }' |
    glab_release api --method POST "projects/${CI_PROJECT_ID}/repository/commits" \
      --header "Content-Type: application/json" --input - >/dev/null

  local mr_args=(
    --repo "$CI_PROJECT_PATH"
    --source-branch "$branch"
    --target-branch "$CI_DEFAULT_BRANCH"
    --title "Release ${tag}"
    --description "Prepares for the ${tag}"$'\n\n'"${entries}"
    --label "$RELEASE_LABELS"
    --remove-source-branch
    --squash-before-merge
    --yes
  )
  if [[ -n "${GITLAB_USER_LOGIN:-}" ]]; then
    mr_args+=(--assignee "$GITLAB_USER_LOGIN")
  fi

  glab_release mr create "${mr_args[@]}"
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
  # dev repositories, and recreating it breaks the mirror sync. An existing tag
  # reachable from this commit is fine (a retry, or a security release synced
  # back with its tags). One that isn't means VERSION names a release tagged
  # elsewhere, so refuse rather than skip.
  if tag_exists "$tag"; then
    if git merge-base --is-ancestor "$tag" "$CI_COMMIT_SHA"; then
      log "Tag ${tag} already exists and is reachable from ${CI_COMMIT_SHA}."
      # The tag may have been created by hand, without a release.
      ensure_release "$tag"
      return 0
    fi

    fail "Tag ${tag} already exists at $(git rev-parse --short "${tag}^{commit}"), which isn't an ancestor of ${CI_COMMIT_SHA}." \
      "Don't move the tag: bump VERSION to the next free version instead."
  fi

  local latest_tag
  latest_tag="$(latest_release_tag)"
  if [[ -n "$latest_tag" && "$(printf '%s\n%s\n' "$latest_tag" "$tag" | sort -V | tail -n1)" != "$tag" ]]; then
    fail "${tag} is lower than the latest release tag ${latest_tag}"
  fi

  local changelog_head
  changelog_head="$(git show "${CI_COMMIT_SHA}:CHANGELOG" | head -n1 | tr -d '[:space:]')"
  [[ "$changelog_head" == "$tag" ]] || fail "CHANGELOG starts with '${changelog_head}', expected '${tag}'"

  # A security release can tag this version after the release merge request
  # was opened, so check again before tagging. When the security release is
  # synced back, its tags are pushed here too and the check above passes.
  ensure_tag_free_in_security_repo "$tag"

  create_release "$tag" "$CI_COMMIT_SHA"
}

## Prints the CHANGELOG section of the tag: the lines between its `vX.Y.Z`
## header and the next one, without surrounding blank lines.
## Arguments: tag, revision to read CHANGELOG from.
changelog_section() {
  git show "$2:CHANGELOG" | awk -v tag="$1" '
    $0 == tag { found = 1; next }
    found && /^v[0-9]+\.[0-9]+\.[0-9]+$/ { exit }
    found { lines[++n] = $0 }
    END {
      first = 1; last = n
      while (first <= last && lines[first] ~ /^[[:space:]]*$/) first++
      while (last >= first && lines[last] ~ /^[[:space:]]*$/) last--
      for (i = first; i <= last; i++) print lines[i]
    }'
}

## Creates the GitLab release for the tag, with its CHANGELOG section as the
## release notes. When a ref is given, the release also creates the annotated
## tag at that ref, which triggers the tag pipeline like a pushed tag.
## Arguments: tag, ref (optional).
create_release() {
  local tag="$1" ref="${2:-}" notes
  notes="$(changelog_section "$tag" "${ref:-$tag}")"
  [[ -n "$notes" ]] || fail "CHANGELOG has no entries for ${tag}"

  local args=(
    "$tag"
    --repo "$CI_PROJECT_PATH"
    --notes "$notes"
    --no-update
  )
  if [[ -n "$ref" ]]; then
    args+=(--ref "$ref" --tag-message "Release ${tag}")
  fi

  if dry_run; then
    log "DRY_RUN: would create release ${tag}${ref:+ and its tag at ${ref}} with these notes:"
    log "$notes"
    return 0
  fi

  glab_release release create "${args[@]}"
}

## Creates the GitLab release for an existing tag, unless it already exists.
ensure_release() {
  local tag="$1" status

  if dry_run && [[ -z "${GITLAB_SHELL_RELEASE_TOKEN:-}" ]]; then
    log "DRY_RUN: would create release ${tag} if it doesn't exist"
    return 0
  fi

  status="$(http_status "${GITLAB_SHELL_RELEASE_TOKEN:?must be set to a project access token with the api scope}" \
    "projects/${CI_PROJECT_ID}/releases/${tag}")"
  case "$status" in
    200)
      log "Release ${tag} already exists, nothing to do."
      ;;
    404)
      create_release "$tag"
      ;;
    *)
      fail "Could not check whether release ${tag} exists (HTTP ${status})."
      ;;
  esac
}

main() {
  case "${1:-}" in
    changelog) cmd_changelog ;;
    prepare) cmd_prepare ;;
    tag) cmd_tag ;;
    *)
      sed -n '3,9p' "$0" | sed 's/^# \{0,1\}//' >&2
      exit 1
      ;;
  esac
}

main "$@"
