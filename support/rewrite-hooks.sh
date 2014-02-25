#!/bin/bash
# This script is deprecated. Use bin/create-hooks instead.

gitlab_shell_dir="$(cd $(dirname $0) && pwd)/.."
exec ${gitlab_shell_dir}/bin/create-hooks
