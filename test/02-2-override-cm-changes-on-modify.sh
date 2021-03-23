#!/usr/bin/env bash

set -e

manifest='apiVersion: v1
kind: Pod
metadata:
  name: 02-2
  namespace: foo
spec:
  containers:
  - image: docker.io/library/alpine:3
    command:
    - tail
    args:
    - -f
    - /dev/null
    name: 02-2
    volumeMounts:
    - mountPath: /mnt
      name: cm-foo
  volumes:
  - csi:
      driver: csi-cm.warm-metal.tech
      volumeAttributes:
        configMap: cm-foo
        commitChangesOn: modify
        conflictPolicy: override
    name: cm-foo
'

echo "$manifest" | kubectl apply --wait -f -

echo "waiting for pod to be ready"
kubectl wait -n foo --for=condition=ready --timeout=10s po/02-2

echo "updating foo.txt"
kubectl -n foo exec 02-2 -- sh -c "echo 'override' > /mnt/foo.txt"

footxt=$(kubectl -n foo get cm cm-foo -o template --template='{{index .data "foo.txt"}}')
if [ "$footxt" != "override" ]; then
  exit 1
fi

echo "updating bar.txt"
kubectl -n foo exec 02-2 -- sh -c "echo 'override' > /mnt/bar.txt"

bartxt=$(kubectl -n foo get cm cm-foo -o template --template='{{index .data "bar.txt"}}')

if [ "$bartxt" != "override" ]; then
  exit 1
fi

echo "Restore configmap foo and bar"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt --from-file=bar.txt | kubectl apply --wait -f -

echo "DONE"

set +e