module github.com/warm-metal/csi-driver-configmap

go 1.16

require (
	github.com/container-storage-interface/spec v1.4.0
	github.com/golang/protobuf v1.5.1 // indirect
	github.com/kubernetes-csi/csi-lib-utils v0.9.1 // indirect
	github.com/warm-metal/csi-drivers v0.5.0-alpha.0.0.20210404173852-9ec9cb097dd2
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/grpc v1.36.1
	k8s.io/api v0.20.5
	k8s.io/apimachinery v0.20.5
	k8s.io/client-go v0.20.5
	k8s.io/klog/v2 v2.4.0
	k8s.io/utils v0.0.0-20201110183641-67b214c5f920
)
