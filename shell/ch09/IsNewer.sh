#!/bin/sh

IsNewer() {
  if [ $# -ne 2 ]; then
    echo "Usage: IsNewer file1 file2" 1>&2
    exit 1
  fi

  if [ ! -f $1 ] || [ ! -f $2 ]; then
    return 1
  fi

  if [ -n "$(find $1 -newer $2 -print)" ]; then
    return 0
  else
    return 1
  fi
}

IsNewer FullName.sh DownShift.sh
