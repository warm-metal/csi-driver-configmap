#!/usr/bin/env bash

set -e

kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt | kubectl apply --wait -f -

manifest='apiVersion: v1
kind: Pod
metadata:
  name: 04-1
  namespace: foo
spec:
  containers:
  - image: docker.io/library/alpine:3
    command:
    - tail
    args:
    - -f
    - /dev/null
    name: 04-1
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
kubectl wait -n foo --for=condition=ready --timeout=10s po/04-1

echo "updating configmap volume"
kubectl -n foo exec 04-1 -- sh -c "yes X | awk '{ printf(\"%s\", \$0)}' | head -c 1048576 > /mnt/foo.txt"
kubectl -n foo exec 04-1 -- sh -c "echo $'\nonlyline' >> /mnt/foo.txt"

echo "unmount the configmap"
kubectl -n foo delete po 04-1

footxt=$(kubectl -n foo get cm cm-foo -o template --template='{{index .data "foo.txt"}}')

if [ "$footxt" != "onlyline" ]; then
  exit 1
fi

echo "Restore configmap foo and bar"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt --from-file=bar.txt | kubectl apply --wait -f -

echo "DONE"

set +e