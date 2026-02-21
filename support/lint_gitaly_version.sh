#!/bin/bash

set -euo pipefail

## Usage:
##    version_allowed gitlab_version gitaly_version
## Returns 0 (success) if the Gitaly version is within the allowed range.
## According to GitLab policy, Gitaly can be at the same minor version (N)
## or one minor version behind (N-1) the current GitLab version.
## Assumes the arguments are sanitized semver versions (major.minor.patch).
version_allowed() {
    local gitlab_version="$1"
    local gitaly_version="$2"

    # Split versions into arrays of major, minor, patch
    IFS='.' read -ra gitlab_semver <<< "$gitlab_version"
    IFS='.' read -ra gitaly_semver <<< "$gitaly_version"

    local gitlab_major=${gitlab_semver[0]:-0}
    local gitlab_minor=${gitlab_semver[1]:-0}
    local gitaly_major=${gitaly_semver[0]:-0}
    local gitaly_minor=${gitaly_semver[1]:-0}

    # Gitaly must be behind GitLab (not equal, not ahead)

    # If Gitaly major < GitLab major: allowed (behind)
    if ((gitaly_major < gitlab_major)); then
        return 0
    fi

    # If Gitaly major > GitLab major: not allowed (ahead)
    if ((gitaly_major > gitlab_major)); then
        return 1
    fi

    # Same major version: Gitaly minor must be less than GitLab minor
    if ((gitaly_minor < gitlab_minor)); then
        return 0
    fi

    # Same version or Gitaly ahead in minor: not allowed
    return 1
}

## Fetches the GitLab version from the remote VERSION file
## Arguments:
##   $1 - Path to temporary file for storing the downloaded version
## Returns:
##   Prints the version string to stdout
##   Returns 1 on error
fetch_gitlab_version() {
    local tmp_file="$1"

    if ! curl -sSf -o "$tmp_file" https://gitlab.com/gitlab-org/gitlab/-/raw/master/VERSION?ref_type=heads; then
        echo "Error: Failed to download GitLab version file" >&2
        return 1
    fi

    local version
    version=$(grep -oE '[0-9]+\.[0-9]+\.[0-9]+' "$tmp_file" | head -1)

    if [[ -z "$version" ]]; then
        echo "Error: Could not parse GitLab version" >&2
        return 1
    fi

    echo "$version"
}

## Parses the Gitaly version from go.mod file
## Arguments:
##   $1 - Path to go.mod file (optional, defaults to ./go.mod)
## Returns:
##   Prints the version string to stdout
##   Returns 1 on error
parse_gitaly_version() {
    local go_mod_path="${1:-./go.mod}"

    if [[ ! -f "$go_mod_path" ]]; then
        echo "Error: go.mod file not found at $go_mod_path" >&2
        return 1
    fi

    local version
    version=$(grep "gitlab.com/gitlab-org/gitaly/v[0-9]" "$go_mod_path" | head -1 | awk '{print $2}' | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1)

    if [[ -z "$version" ]]; then
        echo "Error: Could not parse Gitaly version from $go_mod_path" >&2
        return 1
    fi

    echo "$version"
}

## Main execution logic
## Only runs when the script is executed directly (not sourced)
main() {
    local tmp_file
    tmp_file=$(mktemp)
    trap 'rm -f "$tmp_file"' EXIT

    echo "Fetching current GitLab milestone version..."
    local gitlab_version
    if ! gitlab_version=$(fetch_gitlab_version "$tmp_file"); then
        exit 1
    fi

    local gitaly_version
    if ! gitaly_version=$(parse_gitaly_version); then
        exit 1
    fi

    echo "GitLab version is ${gitlab_version} and Shell is using Gitaly version ${gitaly_version}"

    if version_allowed "$gitlab_version" "$gitaly_version"; then
        echo "✅ The Gitaly version used is allowed!"
        exit 0
    else
        echo "❌ The Gitaly version used is not allowed!"
        echo "GitLab Shell must use Gitaly version ${gitlab_version%.*}.x or one minor version behind"
        echo "See documentation: https://docs.gitlab.com/development/gitaly/#gitaly-version-compatibility-requirement"
        exit 1
    fi
}

# Only run main if script is executed directly (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi

