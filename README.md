# csi-driver-configmap

It is a CSI driver to mount ConfigMap as ephemeral volume. 
Unlike the k8s builtin driver, it focuses on ConfigMap sharing and updating. That is,

1. Sharing ConfigMap between namespaces,
2. Updating ConfigMap while modifying mounted files, or delay the commit action until unmount them,
3. Update local mounted files once ConfigMap modified.