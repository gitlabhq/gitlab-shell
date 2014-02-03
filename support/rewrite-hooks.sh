#!/bin/bash

# $1 is an optional argument specifying the location of the repositories directory.
# Defaults to /home/git/repositories if not provided

src=${1:-"$HOME/repositories"}

function create_link_in {
  ln -s -f "$HOME/gitlab-shell/hooks/update" "$1/hooks/update"
}

for dir in "$src/"*; do
  if [ -d "$dir" ]; then
    if [ "$dir" != "${dir%.git}" ]; then
      create_link_in "$dir"
    else
      for subdir in "$dir/"*.git; do
        create_link_in "$subdir"
      done
    fi
  fi
done
