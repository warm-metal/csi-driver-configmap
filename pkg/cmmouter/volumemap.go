package cmmouter

import (
	"context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

func createVolumeMap(clientset *kubernetes.Clientset, sourceRoot string) *volumeMap {
	volRoot := filepath.Join(sourceRoot, "volumes")
	metaRoot := filepath.Join(sourceRoot, "metadata")
	for _, dir := range []string{volRoot, metaRoot} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			klog.Fatalf("unable to mkdir %q: %s", dir, err)
		}
	}

	volMap := &volumeMap{
		clientset:      clientset,
		volumeRoot:     volRoot,
		metaRoot:       metaRoot,
		metadataMap:    make(map[string]*volumeMetadata),
		volumeHelper:   volumeHelper{volumeRoot: volRoot},
		metadataHelper: metadataHelper{metaRoot: metaRoot},
	}

	volMap.cmWatcher = createCMWatcherMap(clientset, &volMap.volGuard, volMap.updateLocalFs)
	volMap.volWatcher = createVolumeWatcherMap(volRoot, &volMap.volGuard, volMap.commitLocalChanges)
	return volMap
}

type volumeMetadata struct {
	ConfigMapOptions   `json:",inline"`
	ConfigMapName      string `json:"configMapName"`
	ConfigMapNamespace string `json:"configMapNamespace"`
	TargetPath         string `json:"targetPath"`
	Pod                string `json:"pod"`
	PodNamespace       string `json:"podNamespace"`
	ResourceVersion    string `json:"resourceVersion"`
}

type volumeMap struct {
	volumeHelper
	metadataHelper

	clientset  *kubernetes.Clientset
	volumeRoot string
	metaRoot   string

	volGuard sync.Mutex

	// mapping from volumeKey to volumeMetadata
	metadataMap map[string]*volumeMetadata

	cmWatcher  *configMapWatcherMap
	volWatcher *volumeWatcherMap
}

func (m *volumeMap) buildOrDie() {
	m.volGuard.Lock()
	defer m.volGuard.Unlock()

	fis, err := ioutil.ReadDir(m.volumeRoot)
	if err != nil {
		klog.Fatalf("unable to read volumes from %q: %s", m.volumeRoot, err)
	}

	ctx := context.TODO()
	for _, fi := range fis {
		volumeID := fi.Name()
		metadata, err := m.loadMetadata(volumeID)
		if err != nil {
			m.cleanAmbiguousVolume(volumeID, "", "")
			continue
		}

		if err := checkPod(ctx, m.clientset, metadata.Pod, metadata.PodNamespace); err != nil {
			m.cleanAmbiguousVolume(volumeID, "", "")
			continue
		}

		m.metadataMap[volumeID] = metadata

		if err = m.watchVolume(volumeID, metadata); err != nil {
			m.cleanAmbiguousVolume(volumeID, metadata.ConfigMapName, metadata.ConfigMapNamespace)
			continue
		}
	}

	// clean dangling metadata
	metadatafis, err := ioutil.ReadDir(m.metaRoot)
	if err != nil {
		klog.Fatalf("unable to read metadata from %q: %s", m.metaRoot, err)
	}

	for _, fi := range metadatafis {
		_, err := os.Lstat(filepath.Join(m.volumeRoot, fi.Name()))
		if err == nil {
			continue
		}

		if !os.IsNotExist(err) {
			klog.Fatalf("unable to access volume %q: %s", fi.Name(), err)
		}

		m.deleteMetadata(fi.Name())
	}
}

func (m *volumeMap) watchVolume(volumeID string, metadata *volumeMetadata) error {
	if metadata.KeepCurrentAlways {
		// watch changes on the configmap and update local volumes
		if err := m.cmWatcher.watchCM(volumeID, metadata.ConfigMapName, metadata.ConfigMapNamespace); err != nil {
			return err
		}
	}

	if metadata.CommitChangesOn == CommitOnModify {
		klog.Infof("local modification of volume %q is going to sync to configmap %s/%s", volumeID,
			metadata.ConfigMapNamespace, metadata.ConfigMapName)
		if err := m.volWatcher.watchVolume(volumeID, len(metadata.SubPath) == 0); err != nil {
			return err
		}
	}

	return nil
}

func checkPod(ctx context.Context, clientset *kubernetes.Clientset, podName, podNS string) error {
	_, err := clientset.CoreV1().Pods(podNS).Get(ctx, podName,  metav1.GetOptions{})
	if err != nil {
		klog.Errorf("unable to fetch pod %s/%s: %s", podNS, podName, err)
		return err
	}

	return nil
}

func (m *volumeMap) cleanAmbiguousVolume(volumeID, cmName, cmNamespace string) {
	klog.Errorf(">> clear ambiguous resource of volume %q. errors can be ignored", volumeID)
	defer func() {
		klog.Errorf("<< volume %q is removed", volumeID)
	}()

	delete(m.metadataMap, volumeID)
	if len(cmName) > 0 {
		m.cmWatcher.unwatchCM(volumeID, cmName, cmNamespace)
	}

	m.volWatcher.unwatchVolume(volumeID, true)
	m.deleteMetadata(volumeID)
	m.deleteVolume(volumeID)
}

func (m *volumeMap) prepareVolume(
	ctx context.Context, volumeID, targetPath, cmName, cmNamespace, pod, podNs string, opts ConfigMapOptions,
) (sourcePath string, err error) {
	cm, err := m.clientset.CoreV1().ConfigMaps(cmNamespace).Get(ctx, cmName, metav1.GetOptions{})
	if err != nil {
		klog.Errorf("unable to fetch configmap %s/%s: %s", cmNamespace, cmName, err)
		err = status.Error(codes.Unavailable, err.Error())
		return
	}

	m.volGuard.Lock()
	defer m.volGuard.Unlock()

	defer func() {
		if err != nil {
			m.cleanAmbiguousVolume(volumeID, cmName, cmNamespace)
		}
	}()

	metadata := &volumeMetadata{
		ConfigMapOptions:   opts,
		ConfigMapName:      cmName,
		ConfigMapNamespace: cmNamespace,
		TargetPath:         targetPath,
		Pod:                pod,
		PodNamespace:       podNs,
	}

	// write local filesystem
	if sourcePath, _, err = m.updateLocalVolume(volumeID, metadata, cm); err != nil {
		return
	}

	if err = m.persistentMetadata(volumeID, metadata); err != nil {
		return
	}

	m.metadataMap[volumeID] = metadata
	if err = m.watchVolume(volumeID, metadata); err != nil {
		return
	}

	klog.Infof("volume %q is ready", volumeID)
	return
}

func (m *volumeMap) unmountVolume(ctx context.Context, volumeID string) (err error) {
	m.volGuard.Lock()
	defer m.volGuard.Unlock()

	metadata := m.metadataMap[volumeID]
	if metadata == nil {
		klog.Fatalf("volume %q is not found. maybe unmounted twice", volumeID)
	}

	delete(m.metadataMap, volumeID)

	if metadata.KeepCurrentAlways {
		m.cmWatcher.unwatchCM(volumeID, metadata.ConfigMapName, metadata.ConfigMapNamespace)
	}

	switch metadata.CommitChangesOn {
	case CommitOnModify:
		m.volWatcher.unwatchVolume(volumeID, len(metadata.SubPath) == 0)
	case CommitOnUnmount:
		m.commitLocalVolumeChanges(volumeID, metadata)
	}

	if err = m.deleteMetadata(volumeID); err != nil {
		return err
	}

	if err = m.deleteVolume(volumeID); err != nil {
		return err
	}

	klog.Infof("volume %q is unmounted", volumeID)
	return nil
}

func (m *volumeMap) updateLocalFs(volumeID string, cm *corev1.ConfigMap) {
	// get volGuard locked in callers
	metadata := m.metadataMap[volumeID]
	if metadata == nil {
		klog.Warningf("volume %q is not found. stop its configmap watcher", volumeID)
		return
	}

	_, updateMetadata, err := m.updateLocalVolume(volumeID, metadata, cm)
	if err == nil && updateMetadata {
		// Ignore the metadata persistent error since that the volume files are up-to-date even the ResourceVersion
		// in the metadata doesn't.
		m.persistentMetadata(volumeID, metadata)
	}

	return
}

func (m *volumeMap) commitLocalChanges(volumeID string) {
	// get volGuard locked in callers
	// FIXME try to commit in background
	metadata := m.metadataMap[volumeID]
	if metadata == nil {
		klog.Warningf("volume %q is not found. stop its configmap watcher", volumeID)
		return
	}

	m.commitLocalVolumeChanges(volumeID, metadata)
}

const configMapSizeHardLimit = 1 << 20

func (m *volumeMap) commitLocalVolumeChanges(volumeID string, metadata *volumeMetadata) {
	volData := m.readLocalVolume(volumeID, metadata)
	if len(volData) == 0 {
		return
	}

	cli := m.clientset.CoreV1().ConfigMaps(metadata.ConfigMapNamespace)

	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		cm, err := cli.Get(context.TODO(), metadata.ConfigMapName, metav1.GetOptions{})
		if err != nil {
			return err
		}

		if cm.ResourceVersion != metadata.ResourceVersion {
			if metadata.ConflictPolicy == DiscardLocalChanges {
				klog.Errorf("remote configmap %s/%s is updated. discard local changes according to the policy",
					metadata.ConfigMapName, metadata.ConfigMapNamespace)
				return nil
			}
		}

		originalSize := 0
		totalSize := 0
		cmData := make(map[string]string, len(cm.Data))
		for k, v := range cm.Data {
			// Users can only update existed values.
			originalSize += len(v)

			if newV, found := volData[k]; found {
				totalSize += len(newV)
				cmData[k] = newV
			} else {
				totalSize += len(v)
				cmData[k] = v
			}
		}

		if totalSize > configMapSizeHardLimit {
			klog.Warningf("total size of updated configmap is over the 1MB limit. apply %q policy",
				metadata.OversizePolicy)
			applyOversizePolicy(cm.Data, volData, originalSize, metadata.OversizePolicy)
		} else {
			cm.Data = cmData
		}

		if cm, err = cli.Update(context.TODO(), cm, metav1.UpdateOptions{}); err != nil {
			klog.Errorf("unable to update configmap for volume %q(size:%d): %s", volumeID, totalSize, err)
			return err
		}

		metadata.ResourceVersion = cm.ResourceVersion
		m.persistentMetadata(volumeID, metadata)
		klog.Infof("volume %q committed", volumeID)
		return nil
	})

	if err != nil {
		klog.Errorf("unable to udpate configmap %s/%s: %s", metadata.ConfigMapNamespace, metadata.ConfigMapName,
			err)
	}
}

func applyOversizePolicy(cmData, volData map[string]string, originalSize int, policy ConfigMapOversizePolicy) {
	fileSizeDelta := make(map[string]int, len(volData))
	deltaOrder := make([]string, 0, len(volData))
	for k, v := range cmData {
		if newV, found := volData[k]; found && newV != v {
			fileSizeDelta[k] = len(newV) - len(v)
			deltaOrder = append(deltaOrder, k)
		}
	}

	sort.Slice(deltaOrder, func(i, j int) bool {
		return fileSizeDelta[deltaOrder[i]] < fileSizeDelta[deltaOrder[j]]
	})

	freeSize := configMapSizeHardLimit - originalSize
	for _, k := range deltaOrder {
		if freeSize < 0 {
			klog.Fatalf("deltaOrder: %#v, k: %s, volData: %#v", deltaOrder, k, volData)
		}

		if fileSizeDelta[k] <= freeSize {
			cmData[k] = volData[k]
			freeSize -= fileSizeDelta[k]
			continue
		}

		maxDataSize := len(cmData[k]) + freeSize
		v := volData[k]
		if len(v) <= maxDataSize {
			klog.Fatalf("k: %s, v: %s, maxDataSize: %d", k, v, maxDataSize)
		}

		switch policy {
		case TruncateHead:
			cmData[k] = v[len(v) - maxDataSize:]
		case TruncateTail:
			cmData[k] = v[:maxDataSize]
		case TruncateHeadLine:
			dataStart := len(v) - maxDataSize
			if dataStart > 0 && v[dataStart-1] != 0x0a {
				lineEnd := strings.IndexByte(v[dataStart:], 0x0a)
				if lineEnd < 0 || lineEnd == maxDataSize {
					continue
				}

				lineEnd++
				dataStart += lineEnd
				freeSize -= len(v) - dataStart - len(cmData[k])
				cmData[k] = v[dataStart:]
				continue
			}

			cmData[k] = v[dataStart:]
		case TruncateTailLine:
			dataEnd := maxDataSize
			if dataEnd < len(v) && v[dataEnd-1] != 0x0a {
				lineEnd := strings.LastIndexByte(v[:dataEnd], 0x0a)
				if lineEnd < 0 {
					continue
				}

				lineEnd++
				freeSize -= lineEnd - len(cmData[k])
				cmData[k] = v[:lineEnd]
				continue
			}

			cmData[k] = v[:dataEnd]
		default:
			panic(policy)
		}

		break
	}
}