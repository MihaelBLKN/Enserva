package network

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrMissingObjectType = errors.New("missing object type")
	ErrMissingObjectID   = errors.New("missing object id")
	ErrObjectNotFound    = errors.New("object not found")
)

type Runtime struct {
	config    Config
	tick      uint64
	objects   map[string]map[string]Object
	factories map[string]ObjectFactory
	mu        sync.RWMutex
	hooksMu   sync.Mutex
}

func NewRuntime(config Config) *Runtime {
	return &Runtime{
		config:    config.Normalized(),
		objects:   map[string]map[string]Object{},
		factories: map[string]ObjectFactory{},
	}
}

func (runtime *Runtime) Config() Config {
	return runtime.config
}

func (runtime *Runtime) Tick() uint64 {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	return runtime.tick
}

func (runtime *Runtime) RegisterObject(object Object) error {
	objectType, objectID, err := objectIdentity(object)
	if err != nil {
		return err
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.objects[objectType] == nil {
		runtime.objects[objectType] = map[string]Object{}
	}
	runtime.objects[objectType][objectID] = object

	return nil
}

func (runtime *Runtime) RemoveObject(objectType, objectID string) {
	objectType = normalizeObjectKey(objectType)
	objectID = normalizeObjectKey(objectID)

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.objects[objectType] == nil {
		return
	}

	delete(runtime.objects[objectType], objectID)
	if len(runtime.objects[objectType]) == 0 {
		delete(runtime.objects, objectType)
	}
}

func (runtime *Runtime) Object(objectType, objectID string) (Object, bool) {
	objectType = normalizeObjectKey(objectType)
	objectID = normalizeObjectKey(objectID)

	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	objectsByID := runtime.objects[objectType]
	if objectsByID == nil {
		return nil, false
	}

	object, ok := objectsByID[objectID]
	return object, ok
}

func (runtime *Runtime) RegisterFactory(objectType string, factory ObjectFactory) error {
	objectType = normalizeObjectKey(objectType)
	if objectType == "" {
		return ErrMissingObjectType
	}
	if factory == nil {
		return errors.New("nil object factory")
	}

	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	runtime.factories[objectType] = factory
	return nil
}

func (runtime *Runtime) Advance() uint64 {
	runtime.hooksMu.Lock()
	defer runtime.hooksMu.Unlock()

	runtime.mu.Lock()
	runtime.tick++
	tick := runtime.tick
	runtime.mu.Unlock()

	ctx := TickContext{
		Tick:         tick,
		Delta:        runtime.config.TickInterval(),
		DeltaSeconds: runtime.config.TickInterval().Seconds(),
		Runtime:      runtime,
	}

	for _, object := range runtime.objectList() {
		if handler, ok := object.(TickHandler); ok {
			handler.OnTick(ctx)
		}
	}

	if tick%uint64(runtime.config.TickRate) == 0 {
		for _, object := range runtime.objectList() {
			if handler, ok := object.(FullTickHandler); ok {
				handler.OnFullTick(ctx)
			}
		}
	}

	return tick
}

func (runtime *Runtime) HandleRequest(ctx RequestContext) error {
	ctx.Request.ObjectType = normalizeObjectKey(ctx.Request.ObjectType)
	ctx.Request.ObjectID = normalizeObjectKey(ctx.Request.ObjectID)
	if ctx.Request.ObjectType == "" {
		return ErrMissingObjectType
	}
	if ctx.Request.ObjectID == "" {
		return ErrMissingObjectID
	}
	if ctx.ReceivedAt.IsZero() {
		ctx.ReceivedAt = time.Now()
	}

	runtime.hooksMu.Lock()
	defer runtime.hooksMu.Unlock()

	runtime.mu.RLock()
	ctx.Tick = runtime.tick
	runtime.mu.RUnlock()
	ctx.Runtime = runtime

	object, err := runtime.objectForRequest(ctx)
	if err != nil {
		return err
	}

	if handler, ok := object.(RequestHandler); ok {
		return handler.OnRequest(ctx)
	}

	return nil
}

func (runtime *Runtime) Snapshot() SnapshotData {
	runtime.hooksMu.Lock()
	defer runtime.hooksMu.Unlock()

	snapshot := SnapshotData{}
	for _, object := range runtime.objectList() {
		objectType, objectID, err := objectIdentity(object)
		if err != nil {
			continue
		}

		if snapshot[objectType] == nil {
			snapshot[objectType] = map[string]any{}
		}
		snapshot[objectType][objectID] = object.Snapshot()
	}

	return snapshot
}

func (runtime *Runtime) objectForRequest(ctx RequestContext) (Object, error) {
	if object, ok := runtime.Object(ctx.Request.ObjectType, ctx.Request.ObjectID); ok {
		return object, nil
	}

	runtime.mu.RLock()
	factory := runtime.factories[ctx.Request.ObjectType]
	runtime.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("%w: %s/%s", ErrObjectNotFound, ctx.Request.ObjectType, ctx.Request.ObjectID)
	}

	object, err := factory.CreateObject(ctx)
	if err != nil {
		return nil, err
	}
	if err := runtime.RegisterObject(object); err != nil {
		return nil, err
	}

	return object, nil
}

func (runtime *Runtime) objectList() []Object {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	objects := make([]Object, 0)
	for _, objectsByID := range runtime.objects {
		for _, object := range objectsByID {
			objects = append(objects, object)
		}
	}

	return objects
}

func objectIdentity(object Object) (string, string, error) {
	if object == nil {
		return "", "", errors.New("nil object")
	}

	objectType := normalizeObjectKey(object.ObjectType())
	if objectType == "" {
		return "", "", ErrMissingObjectType
	}

	objectID := normalizeObjectKey(object.ObjectID())
	if objectID == "" {
		return "", "", ErrMissingObjectID
	}

	return objectType, objectID, nil
}

func normalizeObjectKey(value string) string {
	return strings.TrimSpace(value)
}
