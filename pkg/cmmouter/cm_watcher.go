package cmmouter

import (
	"context"
	"fmt"
	"golang.org/x/xerrors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/util/wait"
	watch2 "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/watch"
	"k8s.io/klog/v2"
	"sync"
)

func createCMWatcherMap(clientset *kubernetes.Clientset, volGuard *sync.Mutex, handler OnConfigMapModify) *configMapWatcherMap {
	ctx, cancel := context.WithCancel(context.TODO())
	return &configMapWatcherMap{
		volGuard:   volGuard,
		watcherMap: make(map[string]*cmWatcherContext),
		updateVol:  handler,
		clientset:  clientset,
		ctx:        ctx,
		cancel:     cancel,
	}
}

type OnConfigMapModify func(volumeKey string, cm *corev1.ConfigMap)

type cmWatcherContext struct {
	volSet map[string]struct{}
	ctx    context.Context
	cancel context.CancelFunc
}

type configMapWatcherMap struct {
	// mapping from mapkey to volumeKeys
	volGuard   *sync.Mutex
	watcherMap map[string]*cmWatcherContext
	updateVol  OnConfigMapModify

	clientset *kubernetes.Clientset
	wg        wait.Group
	ctx       context.Context
	cancel    context.CancelFunc
}

func (m *configMapWatcherMap) watchCM(volumeKey string, cm, ns string) error {
	// should get locked to remove the race condition between unwatchCM and the event handler.
	mapKey := cm + "~" + ns
	klog.Infof("start watching configmap %q for %q", mapKey, volumeKey)
	if watcherCtx, found := m.watcherMap[mapKey]; found {
		klog.Infof("found an existed watch on %q", mapKey)
		if _, found := watcherCtx.volSet[volumeKey]; found {
			panic(fmt.Sprintf("cm %q/%q is already watching for volume %q", ns, cm, volumeKey))
		}

		watcherCtx.volSet[volumeKey] = struct{}{}
		return nil
	}

	listWatcher := cache.NewListWatchFromClient(m.clientset.CoreV1().RESTClient(), "configmaps",
		ns, fields.OneTermEqualSelector("metadata.name", cm),
	)

	watcherCtx := &cmWatcherContext{volSet: map[string]struct{}{volumeKey: {}}}
	watcherCtx.ctx, watcherCtx.cancel = context.WithCancel(m.ctx)
	m.watcherMap[mapKey] = watcherCtx

	m.wg.StartWithContext(watcherCtx.ctx, func(ctx context.Context) {
		_, err := watch.UntilWithSync(
			ctx, listWatcher, &corev1.ConfigMap{}, nil, m.cmEventHandler(mapKey),
		)
		klog.Infof("watch on %q closed: %s", mapKey, err)
	})

	return nil
}

func (m *configMapWatcherMap) unwatchCM(volumeID string, cm, ns string) {
	// should get locked to remove the race condition between unwatchCM and the event handler.

	mapKey := cm + "~" + ns
	watcherCtx, found := m.watcherMap[mapKey]
	if !found {
		klog.Infof("configmap %q is not found in the watcher list", mapKey)
		return
	}

	if _, found := watcherCtx.volSet[volumeID]; !found {
		klog.Infof("volume %q is not found in the configmap watcher list", volumeID)
		return
	}

	klog.Infof("remove volume %q from configmap watch list of %q", volumeID, mapKey)
	delete(watcherCtx.volSet, volumeID)
	if len(watcherCtx.volSet) > 0 {
		return
	}

	klog.Infof("no volume watches on configmap %q. close the watcher", volumeID, mapKey)
	delete(m.watcherMap, mapKey)
	// need not wait for watch loop end
	watcherCtx.cancel()
}

func (m *configMapWatcherMap) cmEventHandler(mapKey string) watch.ConditionFunc {
	return func(event watch2.Event) (done bool, err error) {
		m.volGuard.Lock()
		defer m.volGuard.Unlock()
		watcherCtx := m.watcherMap[mapKey]
		if watcherCtx == nil {
			klog.Infof("no volume is watching configmap %q", mapKey)
			return true, nil
		}

		if len(watcherCtx.volSet) == 0 {
			panic(mapKey)
		}

		switch event.Type {
		case watch2.Error:
			st, ok := event.Object.(*metav1.Status)
			if ok {
				err = xerrors.Errorf("failed %s", st.Message)
			} else {
				err = xerrors.Errorf("unknown error:%#v", event.Object)
			}
		case watch2.Deleted:
			cm := event.Object.(*corev1.ConfigMap)
			relatedVols := make([]string, 0, len(watcherCtx.volSet))
			for vol := range watcherCtx.volSet {
				relatedVols = append(relatedVols, vol)
			}
			klog.Errorf("configmap %q is deleted. volume %#v aren't getting updates anymore.",
				cm.Namespace+"~"+cm.Name, relatedVols)
			done = true
		case watch2.Added:
			klog.Infof("configmap %q is added to the local cache", mapKey)
		case watch2.Modified:
			cm := event.Object.(*corev1.ConfigMap)
			klog.Infof("configmap %s/%s is updated", cm.Namespace, cm.Name)
			for vol := range watcherCtx.volSet {
				klog.Infof("updating volume %q", vol)
				// FIXME considering go in parallel
				m.updateVol(vol, cm)
			}
		default:
			panic(event)
		}

		return
	}
}

func (m *configMapWatcherMap) stop() {
	m.cancel()
	m.wg.Wait()
}
