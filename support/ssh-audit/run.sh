#!/usr/bin/env bash
#
# Boot gitlab-sshd with a defaults-only configuration and run ssh-audit against
# it, either verifying the negotiated algorithms against a committed policy
# (`check`) or regenerating that policy (`make-policy`).
#
# The configuration deliberately does NOT set ciphers/macs/kex_algorithms so we
# exercise the algorithms that are compiled into the binary (which differ
# between FIPS and non-FIPS builds). This is what lets the policy catch an
# algorithm silently appearing or disappearing after a dependency bump.
#
# Requires python3 (to run ssh-audit and pick a free port) and ssh-keygen (to
# generate throwaway host keys).
#
# Usage:
#   support/ssh-audit/run.sh <check|make-policy> <policy-file>
#
# Environment:
#   SSH_AUDIT                 Path to ssh-audit.py (run via python3). If unset,
#                             an `ssh-audit` executable on PATH is used.
#   SSH_AUDIT_HOST_KEY_TYPES  Space-separated host key types to generate.
#                             Defaults to "rsa ecdsa ed25519". ED25519 keys are
#                             not usable under the FIPS crypto module (they are
#                             rejected at load time), so the FIPS job sets this
#                             to "rsa ecdsa".
#   GITLAB_SSHD_BIN           Path to the gitlab-sshd binary. Defaults to
#                             bin/gitlab-sshd in the repository root.
set -euo pipefail

MODE="${1:-}"
POLICY_FILE="${2:-}"

if [[ -z "$MODE" || -z "$POLICY_FILE" ]]; then
  echo "usage: $0 <check|make-policy> <policy-file>" >&2
  exit 2
fi

# Validate the arguments before doing any work (generating keys, booting the
# server) so bad input fails fast.
case "$MODE" in
  check)
    [[ -f "$POLICY_FILE" ]] || { echo "error: policy file $POLICY_FILE not found" >&2; exit 1; }
    ;;
  make-policy) ;;
  *)
    echo "error: unknown mode '$MODE' (use check|make-policy)" >&2
    exit 2
    ;;
esac

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
SSHD_BIN="${GITLAB_SSHD_BIN:-$ROOT_DIR/bin/gitlab-sshd}"
HOST_KEY_TYPES="${SSH_AUDIT_HOST_KEY_TYPES:-rsa ecdsa ed25519}"

command -v python3 >/dev/null 2>&1 || { echo "error: python3 is required" >&2; exit 1; }
command -v ssh-keygen >/dev/null 2>&1 || { echo "error: ssh-keygen is required" >&2; exit 1; }

if [[ -n "${SSH_AUDIT:-}" ]]; then
  SSH_AUDIT_CMD=(python3 "$SSH_AUDIT")
elif command -v ssh-audit >/dev/null 2>&1; then
  SSH_AUDIT_CMD=(ssh-audit)
else
  echo "error: ssh-audit not found; set SSH_AUDIT to ssh-audit.py or install ssh-audit" >&2
  exit 1
fi

[[ -x "$SSHD_BIN" ]] || { echo "error: $SSHD_BIN not found; run 'make compile'" >&2; exit 1; }

WORKDIR="$(mktemp -d)"
SSHD_PID=""
stop_server() {
  if [[ -n "$SSHD_PID" ]]; then
    kill "$SSHD_PID" 2>/dev/null || true
    wait "$SSHD_PID" 2>/dev/null || true
    SSHD_PID=""
  fi
}
cleanup() {
  stop_server
  rm -rf "$WORKDIR"
}
trap cleanup EXIT

host_key_lines=()
for type in $HOST_KEY_TYPES; do
  key_file="$WORKDIR/ssh_host_${type}_key"
  case "$type" in
    rsa)     ssh-keygen -q -t rsa -b 4096 -N '' -f "$key_file" ;;
    ecdsa)   ssh-keygen -q -t ecdsa -b 256 -N '' -f "$key_file" ;;
    ed25519) ssh-keygen -q -t ed25519 -N '' -f "$key_file" ;;
    *) echo "error: unsupported host key type '$type'" >&2; exit 1 ;;
  esac
  host_key_lines+=("    - \"$key_file\"")
done

# start_server picks a free port, writes the config, launches gitlab-sshd and
# waits for it to accept connections. It sets PORT and SSHD_PID and returns:
#   0 - listening
#   1 - the server exited before listening (most likely another process grabbed
#       the port between selection and bind), so the caller should retry
#   2 - the server stayed up but never accepted connections (not retryable)
start_server() {
  PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1", 0)); print(s.getsockname()[1]); s.close()')"

  {
    echo "user: \"git\""
    echo "log_file: \"$WORKDIR/sshd.log\""
    echo "log_format: json"
    echo "secret: \"0123456789abcdef\""
    echo "gitlab_url: \"http://localhost:8080\""
    echo "sshd:"
    echo "  listen: \"127.0.0.1:$PORT\""
    echo "  proxy_protocol: false"
    echo "  web_listen: \"\""
    echo "  host_key_files:"
    printf '%s\n' "${host_key_lines[@]}"
  } > "$WORKDIR/config.yml"

  "$SSHD_BIN" -config-dir "$WORKDIR" &
  SSHD_PID=$!

  for _ in $(seq 1 100); do
    if (exec 3<>"/dev/tcp/127.0.0.1/$PORT") 2>/dev/null; then
      exec 3>&- 3<&-
      return 0
    fi
    if ! kill -0 "$SSHD_PID" 2>/dev/null; then
      return 1
    fi
    sleep 0.1
  done

  return 2
}

started=""
for _ in 1 2 3 4 5; do
  rc=0
  start_server || rc=$?
  if [[ $rc -eq 0 ]]; then
    started=1
    break
  fi

  # Reap the failed attempt's process before retrying or bailing out.
  stop_server

  if [[ $rc -eq 2 ]]; then
    echo "error: gitlab-sshd did not start listening on port $PORT" >&2
    cat "$WORKDIR/sshd.log" >&2 2>/dev/null || true
    exit 1
  fi
  # rc == 1: lost the port race, retry with a fresh port.
done

if [[ -z "$started" ]]; then
  echo "error: gitlab-sshd failed to bind a free port after multiple attempts" >&2
  cat "$WORKDIR/sshd.log" >&2 2>/dev/null || true
  exit 1
fi

if [[ "$MODE" == "make-policy" ]]; then
  "${SSH_AUDIT_CMD[@]}" --skip-rate-test -M "$POLICY_FILE" 127.0.0.1 -p "$PORT"

  # ssh-audit records a host_key_sizes line (and its comment). We do not pin
  # host key sizes (see README), so drop them for a clean, reviewable policy.
  grep -v -e '^host_key_sizes' -e '^# Dictionary containing all host key' \
    "$POLICY_FILE" > "$POLICY_FILE.tmp" && mv "$POLICY_FILE.tmp" "$POLICY_FILE"
else
  "${SSH_AUDIT_CMD[@]}" --skip-rate-test -P "$POLICY_FILE" 127.0.0.1 -p "$PORT"
fi
