#!/usr/bin/env bash

footxt='0
1
2'

bartxt='a
b
c'

function ValidateFile() {
  file=$1
  [ ! -f ${file} ] && exit 1
  content=$(cat ${file})
  case $(basename ${file}) in
  foo.txt)
    if [ "$content" != "$footxt" ]; then
      exit 1
    fi
    ;;
  bar.txt)
    if [ "$content" != "$bartxt" ]; then
      exit 1
    fi
    ;;
  *)
    exit 1
    ;;
  esac
}

set -e
set -x

if [ "${TARGET_FILE}" != "" ]; then
  ValidateFile "${TARGET_FILE}"
fi

if [ "${TARGET_DIR}" != "" ]; then
  [ ! -d "${TARGET_DIR}" ] && exit 1
  for filename in ${TARGET_DIR}/*; do
    ValidateFile "$filename"
  done
fi

set +e
set +x
