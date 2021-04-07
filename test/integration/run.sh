#!/usr/bin/env bash

CASE=$1

echo "Clean legacy resources"
kubectl delete ns foo bar

set -e
echo "Installing the CSI driver..."
kubectl apply -f ../install/csi-driver-cm.yaml

echo "Creating namespace foo and bar"
kubectl create --dry-run=client -oyaml ns foo | kubectl apply --wait -f -
kubectl create --dry-run=client -oyaml ns bar | kubectl apply --wait -f -

echo "Creating configmap foo/cm-foo"
kubectl -n foo create --dry-run=client -oyaml configmap cm-foo --from-file=foo.txt --from-file=bar.txt | kubectl apply --wait -f -

echo "Creating configmap foo/cm-complicated"
kubectl -n foo create --dry-run=client -oyaml configmap cm-complicated \
  --from-file=foo.txt --from-file=foo.bin \
  --from-file=bar.txt --from-file=bar.bin | kubectl apply --wait -f -

echo "Creating configmap bar/cm-bar"
kubectl -n bar create --dry-run=client -oyaml configmap cm-bar --from-file=foo.txt --from-file=bar.txt | kubectl apply --wait -f -

for i in 0*.sh; do
  if [[ "${CASE}" != "" ]]; then
    if [[ "${CASE}" == "$i" ]]; then
      echo "🛠 Run $(basename $i)"
      ./$i
      break
    fi
  else
    echo "🛠 Run $(basename $i)"
    ./$i
  fi
done

set +e
