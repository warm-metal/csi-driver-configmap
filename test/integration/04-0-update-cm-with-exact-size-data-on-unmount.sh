#!/usr/bin/env bash

set -e

kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt | kubectl apply --wait -f -

manifest='apiVersion: v1
kind: Pod
metadata:
  name: 04-0
  namespace: foo
spec:
  containers:
  - image: docker.io/library/alpine:3
    command:
    - tail
    args:
    - -f
    - /dev/null
    name: 04-0
    volumeMounts:
    - mountPath: /mnt/foo.txt
      name: cm-foo
  volumes:
  - csi:
      driver: csi-cm.warm-metal.tech
      volumeAttributes:
        configMap: cm-foo
        subPath: foo.txt
        commitChangesOn: unmount
        conflictPolicy: override
        oversizePolicy: truncateHeadLine
    name: cm-foo
'

echo "$manifest" | kubectl apply --wait -f -

echo "waiting for pod to be ready"
kubectl wait -n foo --for=condition=ready --timeout=10s po/04-0

echo "updating configmap volume"
kubectl -n foo exec 04-0 -- sh -c "base64 /dev/urandom | head -c 1048576 > /mnt/foo.txt"

echo "unmount the configmap"
kubectl -n foo delete po 04-0

footxt=$(kubectl -n foo get cm cm-foo -o template --template='{{index .data "foo.txt"}}')

if [ ${#footxt} -ne 1048576 ]; then
  exit 1
fi

echo "Restore configmap foo and bar"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt --from-file=bar.txt | kubectl apply --wait -f -

echo "DONE"

set +e