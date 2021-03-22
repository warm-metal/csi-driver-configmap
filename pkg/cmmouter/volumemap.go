package cmmouter

import (
	"context"
	"golang.org/x/xerrors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
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
	PodUID             string `json:"podUID"`
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

		if err := checkPod(ctx, m.clientset, metadata.PodUID); err != nil {
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

func checkPod(ctx context.Context, clientset *kubernetes.Clientset, podUID string) error {
	list, err := clientset.CoreV1().Pods(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
		FieldSelector: fields.OneTermEqualSelector("metadata.uid", podUID).String(),
	})

	if err != nil {
		klog.Errorf("unable to list pod %q: %s", podUID, err)
		return err
	}

	if len(list.Items) == 0 {
		klog.Errorf("pod %q not found", podUID)
		return xerrors.Errorf("pod %q not found", podUID)
	}

	return nil
}

func (m *volumeMap) cleanAmbiguousVolume(volumeID, cmName, cmNamespace string)  {
	klog.Errorf("clear ambiguous resource of volume %q. errors can be ignored", volumeID)
	delete(m.metadataMap, volumeID)
	if len(cmName) > 0 {
		m.cmWatcher.unwatchCM(volumeID, cmName, cmNamespace)
	}

	m.volWatcher.unwatchVolume(volumeID, true)
	m.deleteMetadata(volumeID)
	m.deleteVolume(volumeID)
}

func (m *volumeMap) prepareVolume(
	ctx context.Context, volumeID, targetPath, cmName, cmNamespace, podUID string, opts ConfigMapOptions,
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
		PodUID:             podUID,
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

		for k, v := range volData {
			cm.Data[k]  = v
		}

		if cm, err = cli.Update(context.TODO(), cm, metav1.UpdateOptions{}); err != nil {
			return err
		}

		metadata.ResourceVersion = cm.ResourceVersion
		m.persistentMetadata(volumeID, metadata)
		return nil
	})

	if err != nil {
		klog.Errorf("unable to udpate configmap %s/%: %s", metadata.ConfigMapName, metadata.ConfigMapNamespace,
			err)
	}
}
