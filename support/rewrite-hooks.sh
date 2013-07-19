#!/bin/bash

# $1 is an optional argument specifying the location of the repositories directory.
# Defaults to /home/git/repositories if not provided

home_dir="/home/git"
src=${1:-"$home_dir/repositories"}

function create_link_in {
  ln -s -f "$home_dir/gitlab-shell/hooks/update" "$1/hooks/update"
}

for dir in `ls "$src/"`
do
  if [ -d "$src/$dir" ]; then
    if [[ "$dir" =~ ^.*\.git$ ]]
    then
      create_link_in "$src/$dir"
    else
      for subdir in `ls "$src/$dir/"`
      do
        if [ -d "$src/$dir/$subdir" ] && [[ "$subdir" =~ ^.*\.git$ ]]; then
          create_link_in "$src/$dir/$subdir"
        fi
      done
    fi
  fi
done
