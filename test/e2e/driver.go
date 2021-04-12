package main

import (
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kubernetes/test/e2e/framework"
	e2eskipper "k8s.io/kubernetes/test/e2e/framework/skipper"
	"k8s.io/kubernetes/test/e2e/storage/testpatterns"
	"k8s.io/kubernetes/test/e2e/storage/testsuites"
)

type driver struct {
}

func (d driver) GetVolume(config *testsuites.PerTestConfig, volumeNumber int) (attributes map[string]string, shared bool, readOnly bool) {
	return map[string]string{
		"configMap": "cliapp-shell-context",
		"namespace": "cliapp-system",
	}, true, false
}

func (d driver) GetCSIDriverName(config *testsuites.PerTestConfig) string {
	return "csi-cm.warm-metal.tech"
}

func (d driver) GetVolumeSource(readOnly bool, fsType string, testVolume testsuites.TestVolume) *v1.VolumeSource {
	return &v1.VolumeSource{
		CSI: &v1.CSIVolumeSource{
			Driver: "csi-cm.warm-metal.tech",
			VolumeAttributes: map[string]string{
				"configMap": "cliapp-shell-context",
				"namespace": "cliapp-system",
			},
			ReadOnly: &readOnly,
		},
	}
}

type imageVol struct {
}

func (i imageVol) DeleteVolume() {
}

func (d driver) CreateVolume(config *testsuites.PerTestConfig, volumeType testpatterns.TestVolType) testsuites.TestVolume {
	return &imageVol{}
}

func (d driver) GetDriverInfo() *testsuites.DriverInfo {
	return &testsuites.DriverInfo{
		Name: "csi-cm.warm-metal.tech",
		Capabilities: map[testsuites.Capability]bool{
			testsuites.CapExec:          true,
			testsuites.CapMultiPODs:     true,
		},
		SupportedFsType: sets.NewString(""),
	}
}

func (d driver) SkipUnsupportedTest(pattern testpatterns.TestPattern) {
	supported := false
	switch pattern.VolType {
	case testpatterns.InlineVolume, testpatterns.CSIInlineVolume:
		supported = true
	}
	if !supported {
		e2eskipper.Skipf("Driver %q does not support volume type %q - skipping", "csi-cm.warm-metal.tech", pattern.VolType)
	}
}

func (d *driver) PrepareTest(f *framework.Framework) (*testsuites.PerTestConfig, func()) {
	return &testsuites.PerTestConfig{
		Driver:    d,
		Prefix:    "csi-cm",
		Framework: f,
	}, func() {}
}
