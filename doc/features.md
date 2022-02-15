### Discover

Allows users to identify themselves on an instance via SSH. The command is useful for checking out quickly whether a user has SSH access to the instance:

```bash
ssh git@<hostname>

PTY allocation request failed on channel 0
Welcome to GitLab, @username!
Connection to staging.gitlab.com closed.
```

When permission is denied:

```bash
ssh git@<hostname>
git@<hostname>: Permission denied (publickey).
```

### Git operations

Gitlab Shell provides support for Git operations over SSH via processing `git-upload-pack`, `git-receive-pack` and `git-upload-archive` SSH commands. It limit the set of commands to predefined git commands (git push, git clone/pull, git archive).

### Generate new 2FA recovery codes

Allows users to [generate new 2FA recovery codes](https://docs.gitlab.com/ee/user/profile/account/two_factor_authentication.html#generate-new-recovery-codes-using-ssh).

```bash
ssh git@<hostname> 2fa_recovery_codes
Are you sure you want to generate new two-factor recovery codes?
Any existing recovery codes you saved will be invalidated. (yes/no)
yes

Your two-factor authentication recovery codes are:
...
```

### Verify 2FA OTP

Allows users to [verify their 2FA OTP](https://docs.gitlab.com/ee/security/two_factor_authentication.html#2fa-for-git-over-ssh-operations).

```bash
ssh git@<hostname> 2fa_verify
OTP: 347419

OTP validation failed.
```

### LFS authentication

Allows users to generate credentials for LFS authentication.

```bash
ssh git@<hostname> git-lfs-authenticate <project-path> <upload/download>

{"header":{"Authorization":"Basic ..."},"href":"https://gitlab.com/user/project.git/info/lfs","expires_in":7200}
```

### Personal access token

Allows users to personal access tokens via SSH

```bash
ssh git@<hostname> personal_access_token <name> <scope1[,scope2,...]> [ttl_days]

Token:   glpat-...
Scopes:  api
Expires: 2022-02-05
```
