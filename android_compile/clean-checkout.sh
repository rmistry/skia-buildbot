#! /bin/bash

# Bash script to clean an android checkout.

if [ -z "$1" ]
  then
    echo "Missing Android checkout directory"
    exit 1
fi
checkout=$1
cd $checkout

# Delete both index.lock and shallow.lock files.
find . -name index.lock -exec echo "Going to delete " {} \; -exec rm {} \;
find . -name shallow.lock -exec echo "Going to delete " {} \; -exec rm {} \;
