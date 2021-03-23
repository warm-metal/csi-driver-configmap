package cmmouter

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"io/ioutil"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
)

type volumeHelper struct {
	volumeRoot string
}

func (v volumeHelper) updateLocalVolume(
	volumeID string, metadata *volumeMetadata, cm *corev1.ConfigMap,
) (path string, needToPersistentMetadata bool, err error) {
	path = filepath.Join(v.volumeRoot, volumeID)

	if cm.ResourceVersion == metadata.ResourceVersion {
		klog.Infof("ignore the event populated by local volume changes %q - %s", volumeID, cm.ResourceVersion)
		return
	}

	defer func() {
		if err == nil {
			klog.Infof("ResourceVersion of volume %q changes from %q to %q", volumeID, metadata.ResourceVersion,
				cm.ResourceVersion)
			metadata.ResourceVersion = cm.ResourceVersion
			needToPersistentMetadata = true
		}
	}()

	if len(metadata.SubPath) > 0 {
		klog.Infof("update volume file %q", path)
		subContent, found := cm.Data[metadata.SubPath]
		if !found {
			klog.Errorf("subPath %q not found in configmap %s/%s which is mounted by volume %q",
				metadata.SubPath, metadata.ConfigMapNamespace, metadata.ConfigMapName, volumeID)
			err = status.Errorf(codes.NotFound, "subPath %q not found", metadata.SubPath)
			return
		}

		if err = ioutil.WriteFile(path, []byte(subContent), 0644); err != nil {
			klog.Errorf("unable to update volume %q: %s", path, err)
			err = status.Error(codes.Aborted, err.Error())
			return
		}
	} else {
		klog.Infof("update volume directory %q", path)
		if err = os.MkdirAll(path, 0755); err != nil {
			klog.Errorf("unable to create dir %q: %s", path, err)
			err = status.Error(codes.Aborted, err.Error())
			return
		}

		for f, content := range cm.Data {
			subpath := filepath.Join(path, f)
			if err = ioutil.WriteFile(subpath, []byte(content), 0644); err != nil {
				klog.Errorf("unable to update %q: %s", subpath, err)
				err = status.Error(codes.Aborted, err.Error())
				return
			}
		}
	}

	return
}

func (v volumeHelper) readLocalVolume(volumeID string, metadata *volumeMetadata) map[string]string {
	path := filepath.Join(v.volumeRoot, volumeID)
	fi, err := os.Lstat(path)
	if err != nil {
		klog.Errorf("unable to fetch local volume %q: %s", path, err)
		return nil
	}

	if fi.IsDir() {
		if len(metadata.SubPath) > 0 {
			klog.Fatalf("volume %q should be a file with respect to subPath %q", path, metadata.SubPath)
		}

		fis, err := ioutil.ReadDir(path)
		if err != nil {
			klog.Errorf("unable to list local volume %q: %s", path, err)
			return nil
		}

		if len(fis) == 0 {
			klog.Warningf("no files found in local volume %q", path)
			return nil
		}

		data := make(map[string]string, len(fis))
		for _, fi := range fis {
			pathi := filepath.Join(path, fi.Name())
			bytes, err := ioutil.ReadFile(pathi)
			if err != nil {
				klog.Errorf("unable to read local volume %q: %s", pathi, err)
				return nil
			}

			data[fi.Name()] = string(bytes)
		}

		return data
	}

	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		klog.Errorf("unable to read local volume %q: %s", path, err)
		return nil
	}

	if len(metadata.SubPath) == 0 {
		klog.Fatalf("volume %q should be a file with respect to any subPath of configmap %s/%s", path,
			metadata.ConfigMapNamespace, metadata.ConfigMapName)
	}

	return map[string]string{metadata.SubPath: string(bytes)}
}

func (v volumeHelper) deleteVolume(volumeID string) error {
	path := filepath.Join(v.volumeRoot, volumeID)
	klog.Infof("remove local volume %q", path)
	err := os.RemoveAll(path)
	if err != nil {
		klog.Errorf("unable to rmdir %q: %s", path, err)
	}
	return err
}
