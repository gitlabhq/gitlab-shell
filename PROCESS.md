## Releasing a new version

GitLab Shell is versioned by git tags, and the version used by the Rails
application is stored in
[`GITLAB_SHELL_VERSION`](https://gitlab.com/gitlab-org/gitlab/-/blob/master/GITLAB_SHELL_VERSION).

For each version, there is a raw version and a tag version:

- The **raw version** is the version number. For instance, `15.2.8`.
- The **tag version** is the raw version prefixed with `v`. For instance, `v15.2.8`.

To release a new version of GitLab Shell and have that version available to the
Rails application:

1. Create a merge request to update the [`CHANGELOG`](CHANGELOG) with the
   **tag version** and the [`VERSION`](VERSION) file with the **raw version**.
2. Ask a maintainer to review and merge the merge request. If you're already a
   maintainer, second maintainer review is not required.
3. Add a new git tag with the **tag version**.
4. Update `GITLAB_SHELL_VERSION` in the Rails application to the **raw
   version**. (Note: this can be done as a separate MR to that, or in and MR
   that will make use of the latest GitLab Shell changes.)

## Security releases

GitLab Shell is included in the packages we create for GitLab, and each version of GitLab specifies the version of GitLab Shell it uses in the `GITLAB_SHELL_VERSION` file, so security fixes in GitLab Shell are tightly coupled to the [GitLab security release](https://about.gitlab.com/handbook/engineering/workflow/#security-issues) workflow.

For a security fix in GitLab Shell, two sets of merge requests are required:

* The fix itself, in the `gitlab-org/security/gitlab-shell` repository and its backports to the previous versions of GitLab Shell
* Merge requests to change the versions of GitLab Shell included in the GitLab security release, in the `gitlab-org/security/gitlab` repository

The first step could be to create a merge request with a fix targeting `main` in `gitlab-org/security/gitlab-shell`. When the merge request is approved by maintainers, backports targeting previous 3 versions of GitLab Shell must be created. The stable branches for those versions may not exist, feel free to ask a maintainer to create ones. The stable branches must be created out of the GitLab Shell tags/version used by the 3 previous GitLab releases. In order to find out the GitLab Shell version that is used on a particular GitLab stable release, the following steps may be helpful:

```shell
git fetch security 13-9-stable-ee
git show refs/remotes/security/13-9-stable-ee:GITLAB_SHELL_VERSION
```

These steps display the version that is used by `13.9` version of GitLab.

Close to the GitLab security release, a maintainer should merge the fix and backports and cut all the necessary GitLab Shell versions. This allows bumping the `GITLAB_SHELL_VERSION` for `gitlab-org/security/gitlab`. The GitLab merge request will be handled by the general GitLab security release process.

Once the security release is done, a GitLab Shell maintainer is responsible for syncing tags and `main` to the `gitlab-org/gitlab-shell` repository.
