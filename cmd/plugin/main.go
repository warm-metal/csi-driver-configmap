package main

import (
	"flag"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/warm-metal/csi-driver-configmap/pkg/cmmouter"
	"github.com/warm-metal/csi-drivers/pkg/csi-common"
	"k8s.io/klog/v2"
)

var (
	endpoint   = flag.String("endpoint", "unix:///csi/csi.sock", "endpoint")
	nodeID     = flag.String("node", "", "node ID")
	sourceRoot = flag.String("cm-source-root", "/var/lib/warm-metal/cm-volume",
		"Directory to save directories and files populated from ConfigMaps")
)

const (
	driverName    = "csi-cm.warm-metal.tech"
	driverVersion = "v1.0.0"
)

func main() {
	klog.InitFlags(nil)
	if err := flag.Set("logtostderr", "true"); err != nil {
		panic(err)
	}

	defer klog.Flush()
	flag.Parse()
	driver := csicommon.NewCSIDriver(driverName, driverVersion, *nodeID)
	driver.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
	})
	driver.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_UNKNOWN,
	})

	server := csicommon.NewNonBlockingGRPCServer()

	server.Start(*endpoint,
		csicommon.NewDefaultIdentityServer(driver),
		&controllerServer{csicommon.NewDefaultControllerServer(driver)},
		&nodeServer{
			DefaultNodeServer: csicommon.NewDefaultNodeServer(driver),
			mounter:           cmmouter.NewMounterOrDie(*sourceRoot),
		},
	)
	server.Wait()
}
