#!/usr/bin/env bash

set -e
manifest='apiVersion: batch/v1
kind: Job
metadata:
  name: 00-1
  namespace: foo
spec:
  template:
    metadata:
      name: 00-1
    spec:
      containers:
        - name: 00-1
          image: docker.io/warmmetal/csi-configmap-test:v0.1.0
          env:
          - name: TARGET_DIR
            value: /mnt
          volumeMounts:
            - mountPath: /mnt
              name: cm-bar
      restartPolicy: Never
      volumes:
        - name: cm-bar
          csi:
            driver: csi-cm.warm-metal.tech
            volumeAttributes:
              configMap: cm-bar
              namespace: bar
  backoffLimit: 0
'

echo "$manifest" | kubectl apply --wait -f -

echo "waiting for job complete"
kubectl wait -n foo --for=condition=complete --timeout=10s job/00-1

succeeded=$(kubectl -n foo get job  00-1 -o template --template={{.status.succeeded}})
if [ "$succeeded" != "1" ]; then
  echo "Job doesn't succeed in 10s"
  kubectl -n foo get job 00-1 -oyaml
  exit 1
fi

echo "DONE"

set +e