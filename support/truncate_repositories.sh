#!/bin/bash

home_dir="/home/git"

echo "Danger!!! Data Loss"
while true; do
  read -p "Do you wish to delete all directories (except gitolite-admin.git) from $home_dir/repositories/ (y/n) ?:  " yn
  case $yn in
    [Yy]* ) sh -c "find $home_dir/repositories/. -maxdepth 1  -not -name 'gitolite-admin.git' -not -name '.' | xargs rm -rf"; break;;
    [Nn]* ) exit;;
    * ) echo "Please answer yes or no.";;
  esac
done
