#!/bin/bash

# $1 is an optional argument specifying the location of the repositories directory.
# Defaults to /home/git/repositories if not provided


home_dir="/home/git/repositories"
src=${1:-"$home_dir"}

echo "Danger!!! Data Loss"
while true; do
  read -p "Do you wish to delete all directories from $home_dir/ (y/n) ?:  " yn
  case $yn in
    [Yy]* ) sh -c "find $home_dir/. -maxdepth 1 -not -name '.' | xargs rm -rf"; break;;
    [Nn]* ) exit;;
    * ) echo "Please answer yes or no.";;
  esac
done
