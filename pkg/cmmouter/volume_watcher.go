package cmmouter

import (
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"k8s.io/utils/inotify"
	"path/filepath"
	"sync"
)

func createVolumeWatcherMap(volRoot string, volGuard *sync.Mutex, handleChange volumeModifiedHandle) *volumeWatcherMap {
	volMap := &volumeWatcherMap{
		volumeRoot:   volRoot,
		volGuard:     volGuard,
		watcherMap:   make(map[string]struct{}),
		handleChange: handleChange,
	}

	var err error
	volMap.fsWatcher, err = inotify.NewWatcher()
	if err != nil {
		klog.Fatal(err)
	}

	volMap.wg.Start(volMap.evLoop)
	return volMap
}

type volumeModifiedHandle func(volumeKey string)

type volumeWatcherMap struct {
	volumeRoot string
	volGuard   *sync.Mutex
	// mapping save volumeKeys
	watcherMap   map[string]struct{}
	handleChange volumeModifiedHandle

	fsWatcher *inotify.Watcher
	wg        wait.Group
}

func (m *volumeWatcherMap) watchVolume(volumeID string, dir bool) (err error) {
	if _, found := m.watcherMap[volumeID]; found {
		panic(volumeID)
	}

	m.watcherMap[volumeID] = struct{}{}
	defer func() {
		if err != nil {
			delete(m.watcherMap, volumeID)
		}
	}()

	if !dir {
		klog.Infof("volume %q is watching the volume root", volumeID)
		if len(m.watcherMap) == 1 {
			klog.Infof("start inotify on the volume root %q", m.volumeRoot)
			if err = m.fsWatcher.Watch(m.volumeRoot); err != nil {
				klog.Errorf("unable to watch %q: %s", m.volumeRoot, err)
				return
			}
		}

		return
	}

	path := filepath.Join(m.volumeRoot, volumeID)
	klog.Infof("volume %q is watching dir %q", volumeID, path)
	if err = m.fsWatcher.Watch(path); err != nil {
		klog.Errorf("unable to watch %q: %s", path, err)
		return
	}

	return
}

func (m *volumeWatcherMap) unwatchVolume(volumeID string, dir bool) error {
	if _, found := m.watcherMap[volumeID]; !found {
		klog.Infof("volume %q is not found in the inotify list", volumeID)
		return nil
	}

	klog.Infof("remove inotify watch for volume %q", volumeID)
	delete(m.watcherMap, volumeID)
	if dir {
		path := filepath.Join(m.volumeRoot, volumeID)
		err := m.fsWatcher.RemoveWatch(path)
		if err != nil {
			klog.Errorf("unable to remove inotify on %q: %s", path, err)
		}
		return err
	}

	if len(m.watcherMap) == 0 {
		klog.Infof("remove inotify on the volume root %q", m.volumeRoot)
		err := m.fsWatcher.RemoveWatch(m.volumeRoot)
		if err != nil {
			klog.Errorf("unable to remove inotify on the volume root %q: %s", m.volumeRoot, err)
		}

		return err
	}

	return nil
}

func (m *volumeWatcherMap) evLoop() {
	for {
		select {
		case event, ok := <-m.fsWatcher.Event:
			if !ok {
				return
			}

			if m.handleChange == nil {
				klog.Warning("no handle found")
				break
			}

			klog.Infof("fs event: %#v", event)
			if len(event.Name) < len(m.volumeRoot) {
				panic(event.Name)
			}

			if event.Mask&inotify.InCloseWrite != inotify.InCloseWrite {
				klog.V(1).Infof("ignore event %s", event)
				break
			}

			dir, file := filepath.Split(event.Name[len(m.volumeRoot)+1:])
			var volumeID string
			if len(dir) == 0 {
				volumeID = file
			} else {
				volumeID = dir[:len(dir)-1]
			}

			klog.Infof("fs events of volume %q", volumeID)
			m.volGuard.Lock()
			if _, found := m.watcherMap[volumeID]; found {
				m.handleChange(volumeID)
			}
			m.volGuard.Unlock()

		case err, ok := <-m.fsWatcher.Error:
			if !ok {
				return
			}
			klog.Error(err)
		}
	}
}

func (m *volumeWatcherMap) stop() {
	m.fsWatcher.Close()
	m.wg.Wait()
}
