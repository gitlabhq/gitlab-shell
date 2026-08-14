# ssh-audit algorithm policies

gitlab-sshd advertises a set of key exchanges, ciphers, MACs and host key
algorithms that is derived from the Go standard library, `golang.org/x/crypto`
and labkit. A dependency bump can quietly add or drop one of these, as happened
in [MR 1524](https://gitlab.com/gitlab-org/gitlab-shell/-/merge_requests/1524).
The unit tests in `internal/sshd/server_config_test.go` compare the running
configuration against the same helper that populates it, so they cannot catch
this class of change.

[`ssh-audit`](https://github.com/jtesta/ssh-audit) connects to a running
gitlab-sshd and reports exactly what it negotiates on the wire. We pin that
output as a policy file and check it in CI, so any change to the offered
algorithms has to be an explicit, reviewed update to the policy.

## Files

| File | Description |
| --- | --- |
| `run.sh` | Boots gitlab-sshd with a defaults-only config and runs ssh-audit against it (`check` or `make-policy`). |
| `gitlab-sshd.policy` | Golden algorithm snapshot for the default (non-FIPS) build. |
| `gitlab-sshd-fips.policy` | Golden algorithm snapshot for the FIPS build. |

The config used by `run.sh` deliberately leaves `ciphers`, `macs` and
`kex_algorithms` unset so the binary's compiled-in defaults are exercised — that
is where FIPS and non-FIPS builds differ and where a dependency-driven change
would show up.

## Running locally

```sh
make ssh-audit-test
```

This builds gitlab-sshd, downloads a pinned ssh-audit, and verifies the default
build against `gitlab-sshd.policy`. `python3` and `ssh-keygen` must be
available.

## Updating a policy after an intentional change

When you deliberately change the offered algorithms, regenerate the relevant
policy and commit it in the same change so reviewers see the diff.

Non-FIPS:

```sh
make ssh-audit-generate-policy
```

FIPS (run in a FIPS build environment such as the CI FIPS image; a FIPS binary
cannot be built on non-Linux/amd64 hosts):

```sh
make ssh-audit-generate-policy FIPS_MODE=1 \
  SSH_AUDIT_POLICY=support/ssh-audit/gitlab-sshd-fips.policy \
  SSH_AUDIT_HOST_KEY_TYPES="rsa ecdsa"
```

The FIPS build omits the ed25519 host key on purpose: ED25519 keys are not
usable under the FIPS crypto module and are rejected at load time
(`ED25519 keys are not allowed in FIPS mode`).

Review the regenerated diff carefully — the point of the check is that removing
an algorithm is a deliberate, visible decision.
