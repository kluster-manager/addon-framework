package factory

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/cache"
)

// DefaultQueueKey is the queue key used for string trigger based controllers.
const DefaultQueueKey = "key"

// DefaultQueueKeysFunc returns a slice with a single element - the DefaultQueueKey
func DefaultQueueKeysFunc(_ runtime.Object) []string {
	return []string{DefaultQueueKey}
}

// Factory is generator that generate standard Kubernetes controllers.
// Factory is really generic and should be only used for simple controllers that does not require special stuff..
type Factory struct {
	sync              SyncFunc
	syncContext       SyncContext
	resyncInterval    time.Duration
	informers         []filteredInformers
	informerQueueKeys []informersWithQueueKey
	bareInformers     []Informer
	cachesToSync      []cache.InformerSynced
}

// Informer represents any structure that allow to register event handlers and informs if caches are synced.
// Any SharedInformer will comply.
type Informer interface {
	AddEventHandler(handler cache.ResourceEventHandler)
	HasSynced() bool
}

type informersWithQueueKey struct {
	informers  []Informer
	filter     EventFilterFunc
	queueKeyFn ObjectQueueKeysFunc
}

type filteredInformers struct {
	informers []Informer
	filter    EventFilterFunc
}

// ObjectQueueKeysFunc is used to make a string work queue keys out of the runtime object that is passed to it.
// This can extract the "namespace/name" if you need to or just return "key" if you building controller that only use string
// triggers.
type ObjectQueueKeysFunc func(runtime.Object) []string

// EventFilterFunc is used to filter informer events to prevent Sync() from being called
type EventFilterFunc func(obj interface{}) bool

// New return new factory instance.
func New() *Factory {
	return &Factory{}
}

// Sync is used to set the controller synchronization function. This function is the core of the controller and is
// usually hold the main controller logic.
func (f *Factory) WithSync(syncFn SyncFunc) *Factory {
	f.sync = syncFn
	return f
}

// WithInformers is used to register event handlers and get the caches synchronized functions.
// Pass informers you want to use to react to changes on resources. If informer event is observed, then the Sync() function
// is called.
func (f *Factory) WithInformers(informers ...Informer) *Factory {
	f.WithFilteredEventsInformers(nil, informers...)
	return f
}

// WithFilteredEventsInformers is used to register event handlers and get the caches synchronized functions.
// Pass the informers you want to use to react to changes on resources. If informer event is observed, then the Sync() function
// is called.
// Pass filter to filter out events that should not trigger Sync() call.
func (f *Factory) WithFilteredEventsInformers(filter EventFilterFunc, informers ...Informer) *Factory {
	f.informers = append(f.informers, filteredInformers{
		informers: informers,
		filter:    filter,
	})
	return f
}

// WithBareInformers allow to register informer that already has custom event handlers registered and no additional
// event handlers will be added to this informer.
// The controller will wait for the cache of this informer to be synced.
// The existing event handlers will have to respect the queue key function or the sync() implementation will have to
// count with custom queue keys.
func (f *Factory) WithBareInformers(informers ...Informer) *Factory {
	f.bareInformers = append(f.bareInformers, informers...)
	return f
}

// WithInformersQueueKeysFunc is used to register event handlers and get the caches synchronized functions.
// Pass informers you want to use to react to changes on resources. If informer event is observed, then the Sync() function
// is called.
// Pass the queueKeyFn you want to use to transform the informer runtime.Object into string key used by work queue.
func (f *Factory) WithInformersQueueKeysFunc(queueKeyFn ObjectQueueKeysFunc, informers ...Informer) *Factory {
	f.informerQueueKeys = append(f.informerQueueKeys, informersWithQueueKey{
		informers:  informers,
		queueKeyFn: queueKeyFn,
	})
	return f
}

// WithFilteredEventsInformersQueueKeysFunc is used to register event handlers and get the caches synchronized functions.
// Pass informers you want to use to react to changes on resources. If informer event is observed, then the Sync() function
// is called.
// Pass the queueKeyFn you want to use to transform the informer runtime.Object into string key used by work queue.
// Pass filter to filter out events that should not trigger Sync() call.
func (f *Factory) WithFilteredEventsInformersQueueKeysFunc(queueKeyFn ObjectQueueKeysFunc, filter EventFilterFunc, informers ...Informer) *Factory {
	f.informerQueueKeys = append(f.informerQueueKeys, informersWithQueueKey{
		informers:  informers,
		filter:     filter,
		queueKeyFn: queueKeyFn,
	})
	return f
}

// ResyncEvery will cause the Sync() function to be called periodically, regardless of informers.
// This is useful when you want to refresh every N minutes or you fear that your informers can be stucked.
// If this is not called, no periodical resync will happen.
// Note: The controller context passed to Sync() function in this case does not contain the object metadata or object itself.
//
//	This can be used to detect periodical resyncs, but normal Sync() have to be cautious about `nil` objects.
func (f *Factory) ResyncEvery(interval time.Duration) *Factory {
	f.resyncInterval = interval
	return f
}

// WithSyncContext allows to specify custom, existing sync context for this factory.
// This is useful during unit testing where you can override the default event recorder or mock the runtime objects.
// If this function not called, a SyncContext is created by the factory automatically.
func (f *Factory) WithSyncContext(ctx SyncContext) *Factory {
	f.syncContext = ctx
	return f
}

// Controller produce a runnable controller.
func (f *Factory) ToController(name string) Controller {
	if f.sync == nil {
		panic(fmt.Errorf("WithSync() must be used before calling ToController() in %q", name))
	}

	var ctx SyncContext
	if f.syncContext != nil {
		ctx = f.syncContext
	} else {
		ctx = NewSyncContext(name)
	}

	c := &baseController{
		name:             name,
		sync:             f.sync,
		resyncEvery:      f.resyncInterval,
		cachesToSync:     append([]cache.InformerSynced{}, f.cachesToSync...),
		syncContext:      ctx,
		cacheSyncTimeout: defaultCacheSyncTimeout,
	}

	for i := range f.informerQueueKeys {
		for d := range f.informerQueueKeys[i].informers {
			informer := f.informerQueueKeys[i].informers[d]
			queueKeyFn := f.informerQueueKeys[i].queueKeyFn
			informer.AddEventHandler(c.syncContext.(syncContext).eventHandler(queueKeyFn, f.informerQueueKeys[i].filter))
			c.cachesToSync = append(c.cachesToSync, informer.HasSynced)
		}
	}

	for i := range f.informers {
		for d := range f.informers[i].informers {
			informer := f.informers[i].informers[d]
			informer.AddEventHandler(c.syncContext.(syncContext).eventHandler(DefaultQueueKeysFunc, f.informers[i].filter))
			c.cachesToSync = append(c.cachesToSync, informer.HasSynced)
		}
	}

	for i := range f.bareInformers {
		c.cachesToSync = append(c.cachesToSync, f.bareInformers[i].HasSynced)
	}

	return c
}
