#!/bin/bash

# $1 is an optional argument specifying the location of the repositories directory.
# Defaults to /home/git/repositories if not provided

home_dir="/home/git"
src=${1:-"$home_dir/repositories"}

for dir in `ls "$src/"`
do
  if [ -d "$src/$dir" ]; then
    if [[ "$dir" =~ ^.*\.git$ ]]
    then
      project_hook="$src/$dir/hooks/post-receive"
      gitlab_shell_hook="$home_dir/gitlab-shell/hooks/post-receive"
      ln -s -f $gitlab_shell_hook $project_hook

      project_hook="$src/$dir/hooks/update"
      gitlab_shell_hook="$home_dir/gitlab-shell/hooks/update"
      ln -s -f $gitlab_shell_hook $project_hook
    else
      for subdir in `ls "$src/$dir/"`
      do
        if [ -d "$src/$dir/$subdir" ] && [[ "$subdir" =~ ^.*\.git$ ]]; then
          project_hook="$src/$dir/$subdir/hooks/post-receive"
          gitlab_shell_hook="$home_dir/gitlab-shell/hooks/post-receive"
          ln -s -f $gitlab_shell_hook $project_hook

          project_hook="$src/$dir/$subdir/hooks/update"
          gitlab_shell_hook="$home_dir/gitlab-shell/hooks/update"
          ln -s -f $gitlab_shell_hook $project_hook
        fi
      done
    fi
  fi
done
