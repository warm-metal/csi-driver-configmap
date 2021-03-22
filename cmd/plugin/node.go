package main

import (
	"context"
	"github.com/container-storage-interface/spec/lib/go/csi"
	csicommon "github.com/kubernetes-csi/drivers/pkg/csi-common"
	"github.com/warm-metal/csi-driver-configmap/pkg/cmmouter"
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
	ctxKeyPodNamespace      = "csi.storage.k8s.io/pod.namespace"
	ctxKeyPodUID            = "csi.storage.k8s.io/pod.uid"
)

func (n *nodeServer) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (resp *csi.NodePublishVolumeResponse, err error) {
	klog.Infof("request: %s", req.String())
	ns := req.PublishContext[ctxKeyNamespace]
	if len(ns) == 0 {
		ns = req.PublishContext[ctxKeyPodNamespace]
	}

	err = n.mounter.Mount(ctx, req.VolumeId, req.TargetPath,
		req.PublishContext[ctxKeyConfigMap], ns, req.PublishContext[ctxKeyPodUID],
		cmmouter.ConfigMapOptions{
			SubPath:           req.PublishContext[ctxKeySubPath],
			KeepCurrentAlways: strings.ToLower(req.PublishContext[ctxKeyKeepCurrentAlways]) == "true",
			CommitChangesOn:   cmmouter.ConditionCommitChanges(req.PublishContext[ctxKeyCommitChangesOn]),
			ConflictPolicy:    cmmouter.ConfigMapConflictPolicy(req.PublishContext[ctxKeyConflictPolicy]),
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
