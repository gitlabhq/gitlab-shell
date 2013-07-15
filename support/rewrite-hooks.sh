#!/bin/bash

# $1 is an optional argument specifying the location of the repositories directory.
# Defaults to /home/git/repositories if not provided

home_dir="/home/git"
src=${1:-"$home_dir/repositories"}

function create_links_in {
  for gitlab_shell_hooks in ${home_dir}/gitlab-shell/hooks/* ; do
    ln -s -f "$gitlab_shell_hooks" "$1/hooks/"
  done
}

for dir in `ls "$src/"`
do
  if [ -d "$src/$dir" ]; then
    if [[ "$dir" =~ ^.*\.git$ ]]
    then
      create_links_in "$src/$dir"
    else
      for subdir in `ls "$src/$dir/"`
      do
        if [ -d "$src/$dir/$subdir" ] && [[ "$subdir" =~ ^.*\.git$ ]]; then
          create_links_in "$src/$dir/$subdir"
        fi
      done
    fi
  fi
done
