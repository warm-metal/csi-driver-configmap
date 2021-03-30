#!/usr/bin/env bash

set -e

kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt | kubectl apply --wait -f -

manifest='apiVersion: v1
kind: Pod
metadata:
  name: 05-0
  namespace: foo
spec:
  containers:
  - image: docker.io/library/alpine:3
    command:
    - tail
    args:
    - -f
    - /dev/null
    name: 05-0
    volumeMounts:
    - mountPath: /mnt
      name: cm-complicated
  volumes:
  - csi:
      driver: csi-cm.warm-metal.tech
      volumeAttributes:
        configMap: cm-complicated
        commitChangesOn: unmount
        conflictPolicy: override
        oversizePolicy: truncateHeadLine
    name: cm-complicated
'

echo "$manifest" | kubectl apply --wait -f -

echo "waiting for pod to be ready"
kubectl wait -n foo --for=condition=ready --timeout=10s po/05-0

echo "updating configmap volume"
kubectl -n foo exec 05-0 -- sh -c "echo 'foo.txt' > /mnt/foo.txt"
kubectl -n foo exec 05-0 -- sh -c "echo 'bar.txt' > /mnt/bar.txt"
kubectl -n foo exec 05-0 -- sh -c "echo 'foo.bin' > /mnt/foo.bin"
kubectl -n foo exec 05-0 -- sh -c "echo 'bar.bin' > /mnt/bar.bin"

echo "unmount the configmap"
kubectl -n foo delete po 05-0

footxt=$(kubectl -n foo get cm cm-complicated -o template --template='{{index .data "foo.txt"}}')
if [ "$footxt" != "foo.txt" ]; then
  exit 1
fi

bartxt=$(kubectl -n foo get cm cm-complicated -o template --template='{{index .data "bar.txt"}}')
if [ "$bartxt" != "bar.txt" ]; then
  exit 1
fi

foobin=$(kubectl -n foo get cm cm-complicated -o template --template='{{index .binaryData "foo.bin"}}')
if [ "$foobin" != "$(echo foo.bin | base64)" ]; then
  exit 1
fi

barbin=$(kubectl -n foo get cm cm-complicated -o template --template='{{index .binaryData "bar.bin"}}')
if [ "$barbin" != "$(echo bar.bin | base64)" ]; then
  exit 1
fi

echo "Restore configmap complicated"
kubectl -n foo create --dry-run=client -oyaml configmap cm-complicated \
  --from-file=foo.txt --from-file=foo.bin \
  --from-file=bar.txt --from-file=bar.bin | kubectl apply --wait -f -

echo "DONE"

set +e