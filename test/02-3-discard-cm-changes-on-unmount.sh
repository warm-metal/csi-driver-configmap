#!/usr/bin/env bash

set -e

manifest='apiVersion: v1
kind: Pod
metadata:
  name: 02-3
  namespace: foo
spec:
  containers:
  - image: docker.io/library/alpine:3
    command:
    - tail
    args:
    - -f
    - /dev/null
    name: 02-3
    volumeMounts:
    - mountPath: /mnt
      name: cm-foo
  volumes:
  - csi:
      driver: csi-cm.warm-metal.tech
      volumeAttributes:
        configMap: cm-foo
        commitChangesOn: unmount
        conflictPolicy: discard
    name: cm-foo
'

echo "$manifest" | kubectl apply --wait -f -

echo "waiting for pod to be ready"
kubectl wait -n foo --for=condition=ready --timeout=10s po/02-3

echo "updating configmap foo/cm-foo"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt=foo-v2.txt --from-file=bar.txt=bar-v2.txt | kubectl apply --wait -f -

footxtv1='0
1
2'

bartxtv1='a
b
c'

footxt=$(kubectl -n foo exec 02-3 -- cat /mnt/foo.txt)
bartxt=$(kubectl -n foo exec 02-3 -- cat /mnt/bar.txt)

if [ "$footxt" != "$footxtv1" ]; then
  exit 1
fi

if [ "$bartxt" != "$bartxtv1" ]; then
  exit 1
fi

echo "updating configmap volume"
kubectl -n foo exec 02-3 -- sh -c "echo 'override' > /mnt/foo.txt"

echo "unmount the configmap"
kubectl -n foo delete po 02-3

footxtv2='2
1
0'

bartxtv2='c
b
a'

cmfootxt=$(kubectl -n foo get cm cm-foo -o template --template='{{index .data "foo.txt"}}')
cmbartxt=$(kubectl -n foo get cm cm-foo -o template --template='{{index .data "bar.txt"}}')

if [ "$cmfootxt" != "$footxtv2" ]; then
  exit 1
fi

if [ "$cmbartxt" != "$bartxtv2" ]; then
  exit 1
fi

echo "Restore configmap foo and bar"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt --from-file=bar.txt | kubectl apply --wait -f -

echo "DONE"

set +e