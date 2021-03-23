# csi-driver-configmap

It is a CSI driver to mount ConfigMap as ephemeral volume. 
Unlike the k8s builtin driver, it focuses on ConfigMap sharing and updating. That is,

1. Sharing ConfigMap between namespaces,
2. Updating ConfigMap while modifying mounted files, or delay the commit action until unmount the local volume,
3. Stay current with the ConfigMap if it is updated by other clients.

## Installation
```shell script
kubectl apply -f https://raw.githubusercontent.com/warm-metal/csi-driver-configmap/master/install/csi-driver-cm.yaml
```

## Usage
```yaml
apiVersion: v1
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
        # Name of the ConfigMap to be mounted
        configMap: cm-foo

        # Namespace of the ConfigMap. If not set, the current namespace is used.
        namespace: bar
        
        # Same as subPath of the builtin ConfigMap driver
        subPath: foo.txt
        
        # Stay current with the ConfigMap if updated by other clients.
        keepCurrentAlways: "true"
        
        # When to commit changes of the local volume. Valid values are:
        # "" (a blank string), don't commit changes,
        # "unmount", commit changes when unmounting the volume,
        # "modify", commit changes after each modify(on inotify event IN_CLOSE_WRITE).
        commitChangesOn: "unmount"
        
        # Determine how to deal with conflicts while committing local changes.
        # REQUIRED if commitChangesOn is set.
        # Valid values are:
        # "override", override the remote changes if conflicts arise,
        # "discard", discard local changes.
        conflictPolicy: "override"
    name: cm-foo
```

Notice that, even though enabling both `keepCurrentAlways` and `commitChangesOn` for the same volume is supported,
users should avoid getting into this case.
