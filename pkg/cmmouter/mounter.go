package cmmouter

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	"k8s.io/utils/mount"
	"os"
	"path/filepath"
)

type Mounter struct {
	clientset    *kubernetes.Clientset
	cmSourceRoot string

	volumeMap *volumeMap
	mounter   mount.Interface
}

func NewMounterOrDie(sourceRoot string) *Mounter {
	if len(sourceRoot) == 0 || !filepath.IsAbs(sourceRoot) {
		klog.Fatal("--mount-root must be an absolute path")
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		klog.Fatalf("unable to fetch cluster config: %s", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		klog.Fatalf("unable to create k8s clientset: %s", err)
	}

	volRoot := filepath.Join(sourceRoot, "volumes")
	metaRoot := filepath.Join(sourceRoot, "metadata")
	for _, dir := range []string{volRoot, metaRoot} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			klog.Fatalf("unable to mkdir %q: %s", dir, err)
		}
	}

	volMap := createVolumeMap(clientset, sourceRoot)
	volMap.buildOrDie()
	return &Mounter{
		cmSourceRoot: sourceRoot,
		clientset:    clientset,
		volumeMap:    volMap,
		mounter:      mount.New(""),
	}
}

type ConditionCommitChanges string

const (
	NoCommit        ConditionCommitChanges = ""
	CommitOnModify  ConditionCommitChanges = "modify"
	CommitOnUnmount ConditionCommitChanges = "unmount"
)

type ConfigMapConflictPolicy string

const (
	DiscardLocalChanges   ConfigMapConflictPolicy = "discard"
	OverrideRemoteChanges ConfigMapConflictPolicy = "override"
)

type ConfigMapOversizePolicy string

const (
	TruncateHead     ConfigMapOversizePolicy = "truncateHead"
	TruncateHeadLine ConfigMapOversizePolicy = "truncateHeadLine"
	TruncateTail     ConfigMapOversizePolicy = "truncateTail"
	TruncateTailLine ConfigMapOversizePolicy = "truncateTailLine"
)

type ConfigMapOptions struct {
	SubPath           string                  `json:"subPath,omitempty"`
	KeepCurrentAlways bool                    `json:"keepCurrentAlways,omitempty"`
	CommitChangesOn   ConditionCommitChanges  `json:"commitChangesOn,omitempty"`
	ConflictPolicy    ConfigMapConflictPolicy `json:"conflictPolicy,omitempty"`
	OversizePolicy    ConfigMapOversizePolicy `json:"oversizePolicy,omitempty"`
}

func (m *Mounter) Mount(
	ctx context.Context, volumeID, targetPath, cmName, cmNamespace, pod, podNs string, opts ConfigMapOptions, ro bool,
) error {
	if len(volumeID) == 0 {
		return status.Error(codes.InvalidArgument, "missing volumeId")
	}

	if len(targetPath) == 0 {
		return status.Error(codes.InvalidArgument, "missing targetPath")
	}

	if len(cmName) == 0 {
		return status.Error(codes.InvalidArgument, "missing configmap")
	}

	if len(cmNamespace) == 0 {
		return status.Error(codes.InvalidArgument, "missing namespace")
	}

	if len(pod) == 0 {
		return status.Error(codes.InvalidArgument, "missing pod name")
	}

	if len(podNs) == 0 {
		return status.Error(codes.InvalidArgument, "missing pod namespace")
	}

	if notMnt, err := mount.IsNotMountPoint(m.mounter, targetPath); err != nil {
		if !os.IsNotExist(err) {
			return status.Error(codes.Internal, err.Error())
		}

		if len(opts.SubPath) > 0 {
			f, err := os.Create(targetPath)
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}
			f.Close()
		} else {
			if err = os.MkdirAll(targetPath, 0755); err != nil {
				return status.Error(codes.Internal, err.Error())
			}
		}
	} else if !notMnt {
		klog.Warning("%q is already mounted", targetPath)
		return nil
	}

	switch opts.CommitChangesOn {
	case NoCommit:
	case CommitOnModify, CommitOnUnmount:
		switch opts.ConflictPolicy {
		case DiscardLocalChanges, OverrideRemoteChanges:
		default:
			return status.Errorf(codes.InvalidArgument,
				"conflictPolicy is required if commitChangesOn is enabled. valid values are %q and %q",
				DiscardLocalChanges, OverrideRemoteChanges)
		}

		switch opts.OversizePolicy {
		case TruncateHead, TruncateHeadLine, TruncateTail, TruncateTailLine:
		default:
			return status.Errorf(codes.InvalidArgument,
				"oversizePolicy is required if commitChangesOn is enabled. valid values are %q, %q, %q and %q",
				TruncateHead, TruncateHeadLine, TruncateTail, TruncateTailLine)
		}
	default:
		return status.Errorf(codes.InvalidArgument, "valid values of %q are %q, %q, and %q",
			"commitChangesOn", NoCommit, CommitOnModify, CommitOnUnmount)
	}

	source, err := m.volumeMap.prepareVolume(ctx, volumeID, targetPath, cmName, cmNamespace, pod, podNs, opts)
	if err != nil {
		return err
	}

	mountOpts := []string{"rbind"}
	if ro {
		mountOpts = append(mountOpts, "ro")
	}
	return m.mounter.Mount(source, targetPath, "", mountOpts)
}

func (m *Mounter) Unmount(ctx context.Context, volumeID, targetPath string) error {
	if len(volumeID) == 0 {
		return status.Error(codes.InvalidArgument, "missing volumeId")
	}

	if len(targetPath) == 0 {
		return status.Error(codes.InvalidArgument, "missing targetPath")
	}

	if notMnt, err := mount.IsNotMountPoint(m.mounter, targetPath); err != nil {
		return status.Error(codes.Unavailable, err.Error())
	} else if notMnt {
		return nil
	}

	if err := m.mounter.Unmount(targetPath); err != nil {
		return err
	}

	return m.volumeMap.unmountVolume(ctx, volumeID)
}
