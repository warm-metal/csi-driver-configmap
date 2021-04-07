#!/usr/bin/env bash

set -e

manifest='apiVersion: v1
kind: Pod
metadata:
  name: 01-1
  namespace: foo
spec:
  containers:
  - image: docker.io/library/alpine:3
    command:
    - tail
    args:
    - -f
    - /dev/null
    name: 01-1
    volumeMounts:
    - mountPath: /mnt/bar.txt
      name: cm-foo
  volumes:
  - csi:
      driver: csi-cm.warm-metal.tech
      volumeAttributes:
        configMap: cm-foo
        subPath: bar.txt
        keepCurrentAlways: "true"
    name: cm-foo
'

echo "$manifest" | kubectl apply --wait -f -

echo "waiting for pod to be ready"
kubectl wait -n foo --for=condition=ready --timeout=10s po/01-1

echo "updating configmap foo/cm-foo"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt=foo-v2.txt --from-file=bar.txt=bar-v2.txt | kubectl apply --wait -f -

bartxtv2='c
b
a'

bartxt=$(kubectl -n foo exec 01-1 -- cat /mnt/bar.txt)

init=$(date +%s)

while [ "$bartxt" != "$bartxtv2" ]; do
  cur=$(date +%s)
  elapse=$((cur-init))
  if [ $elapse -gt 10 ]; then
    break
  fi

  sleep 1
  bartxt=$(kubectl -n foo exec 01-1 -- cat /mnt/bar.txt)
done

if [ "$bartxt" != "$bartxtv2" ]; then
  exit 1
fi

echo "Restore configmap foo and bar"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt --from-file=bar.txt | kubectl apply --wait -f -

echo "DONE"

set +e