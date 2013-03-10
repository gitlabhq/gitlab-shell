#!/bin/bash

home_dir="/home/git"
src="$home_dir/repositories"

for dir in `ls "$src/"`
do
  if [ -d "$src/$dir" ]; then

    if [ "$dir" = "gitolite-admin.git" ]
    then
      continue 
    fi

    if [[ "$dir" =~ ^.*\.git$ ]]
    then
      project_hook="$src/$dir/hooks/post-receive"
      gitolite_hook="$home_dir/gitlab-shell/hooks/post-receive"
      ln -s -f $gitolite_hook $project_hook

      project_hook="$src/$dir/hooks/update"
      gitolite_hook="$home_dir/gitlab-shell/hooks/update"
      ln -s -f $gitolite_hook $project_hook
    else
      for subdir in `ls "$src/$dir/"`
      do
        if [ -d "$src/$dir/$subdir" ] && [[ "$subdir" =~ ^.*\.git$ ]]; then
          project_hook="$src/$dir/$subdir/hooks/post-receive"
          gitolite_hook="$home_dir/gitlab-shell/hooks/post-receive"
          ln -s -f $gitolite_hook $project_hook

          project_hook="$src/$dir/$subdir/hooks/update"
          gitolite_hook="$home_dir/gitlab-shell/hooks/update"
          ln -s -f $gitolite_hook $project_hook
        fi
      done
    fi
  fi
done
