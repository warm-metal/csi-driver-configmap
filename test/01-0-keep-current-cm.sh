#!/usr/bin/env bash

set -e

manifest='apiVersion: v1
kind: Pod
metadata:
  name: 01-0
  namespace: foo
spec:
  containers:
  - image: docker.io/library/alpine:3
    command:
    - tail
    args:
    - -f
    - /dev/null
    name: 01-0
    volumeMounts:
    - mountPath: /mnt
      name: cm-foo
  volumes:
  - csi:
      driver: csi-cm.warm-metal.tech
      volumeAttributes:
        configMap: cm-foo
        keepCurrentAlways: "true"
    name: cm-foo
'

echo "$manifest" | kubectl apply --wait -f -

echo "waiting for pod to be ready"
kubectl wait -n foo --for=condition=ready --timeout=10s po/01-0

echo "updating configmap foo/cm-foo"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt=foo-v2.txt --from-file=bar.txt=bar-v2.txt | kubectl apply --wait -f -

footxtv2='2
1
0'

bartxtv2='c
b
a'

footxt=$(kubectl -n foo exec 01-0 -- cat /mnt/foo.txt)
bartxt=$(kubectl -n foo exec 01-0 -- cat /mnt/bar.txt)

init=$(date +%s)

while [ "$footxt" != "$footxtv2" ] || [ "$bartxt" != "$bartxtv2" ]; do
  cur=$(date +%s)
  elapse=$((cur-init))
  if [ $elapse -gt 10 ]; then
    break
  fi

  sleep 1
  footxt=$(kubectl -n foo exec 01-0 -- cat /mnt/foo.txt)
  bartxt=$(kubectl -n foo exec 01-0 -- cat /mnt/bar.txt)
done

if [ "$footxt" != "$footxtv2" ]; then
  exit 1
fi

if [ "$bartxt" != "$bartxtv2" ]; then
  exit 1
fi

echo "Restore configmap foo and bar"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt --from-file=bar.txt | kubectl apply --wait -f -

echo "DONE"

set +e