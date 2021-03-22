package cmmouter

import (
	"encoding/json"
	"io/ioutil"
	"k8s.io/klog/v2"
	"os"
	"path/filepath"
)

type metadataHelper struct {
	metaRoot string
}

func (m metadataHelper) loadMetadata(volumeKey string) (*volumeMetadata, error) {
	metadata := volumeMetadata{}
	path := filepath.Join(m.metaRoot, volumeKey)
	bytes, err := ioutil.ReadFile(path)
	if err != nil {
		klog.Errorf("unable to read metadata file %q: %s", path, err)
		return nil, err
	}

	err = json.Unmarshal(bytes, &metadata)
	if err != nil {
		klog.Errorf("unable to decode metadata from %q: %s", path, err)
	}

	return &metadata, err
}

func (m metadataHelper) persistentMetadata(volumeKey string, metadata *volumeMetadata) error {
	bytes, err := json.Marshal(metadata)
	if err != nil {
		klog.Fatalf("unable to marshal metadata %q - %#v: %s", volumeKey, *metadata, err)
		panic(*metadata)
	}

	if err := ioutil.WriteFile(filepath.Join(m.metaRoot, volumeKey), bytes, 0644); err != nil {
		klog.Errorf("unable to write metadata of volume %q: %s", volumeKey, err)
		return err
	}

	return nil
}

func (v metadataHelper) deleteMetadata(volumeID string) error {
	path := filepath.Join(v.metaRoot, volumeID)
	err := os.Remove(path)
	if err != nil {
		klog.Errorf("unable to delete metadata file %q: %s", path, err)
	}
	return err
}