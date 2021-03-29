package main

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/warm-metal/csi-driver-configmap/pkg/cmmouter"
	"github.com/warm-metal/csi-drivers/pkg/csi-common"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
	"strings"
)

type nodeServer struct {
	*csicommon.DefaultNodeServer
	mounter *cmmouter.Mounter
}

func (n nodeServer) NodeStageVolume(context.Context, *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (n nodeServer) NodeUnstageVolume(context.Context, *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (n nodeServer) NodeExpandVolume(context.Context, *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

const (
	ctxKeyConfigMap         = "configMap"
	ctxKeyNamespace         = "namespace"
	ctxKeySubPath           = "subPath"
	ctxKeyKeepCurrentAlways = "keepCurrentAlways"
	ctxKeyCommitChangesOn   = "commitChangesOn"
	ctxKeyConflictPolicy    = "conflictPolicy"
	ctxKeyOversizePolicy    = "oversizePolicy"
	ctxKeyPodNamespace      = "csi.storage.k8s.io/pod.namespace"
	ctxKeyPodName           = "csi.storage.k8s.io/pod.name"
)

func (n *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (resp *csi.NodePublishVolumeResponse, err error) {
	klog.Infof("request: %s", req.String())
	podNs := req.VolumeContext[ctxKeyPodNamespace]
	ns := req.VolumeContext[ctxKeyNamespace]
	if len(ns) == 0 {
		ns = podNs
	}

	err = n.mounter.Mount(ctx, req.VolumeId, req.TargetPath,
		req.VolumeContext[ctxKeyConfigMap], ns, req.VolumeContext[ctxKeyPodName], podNs,
		cmmouter.ConfigMapOptions{
			SubPath:           req.VolumeContext[ctxKeySubPath],
			KeepCurrentAlways: strings.ToLower(req.VolumeContext[ctxKeyKeepCurrentAlways]) == "true",
			CommitChangesOn:   cmmouter.ConditionCommitChanges(req.VolumeContext[ctxKeyCommitChangesOn]),
			ConflictPolicy:    cmmouter.ConfigMapConflictPolicy(req.VolumeContext[ctxKeyConflictPolicy]),
			OversizePolicy:    cmmouter.ConfigMapOversizePolicy(req.VolumeContext[ctxKeyOversizePolicy]),
		})
	if err != nil {
		return
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (n *nodeServer) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (resp *csi.NodeUnpublishVolumeResponse, err error) {
	klog.Infof("request: %s", req.String())
	err = n.mounter.Unmount(ctx, req.VolumeId, req.TargetPath)
	if err != nil {
		return
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}
