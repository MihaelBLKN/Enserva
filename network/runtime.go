package network

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	// ErrMissingObjectType indicates an empty object type.
	ErrMissingObjectType = errors.New("missing object type")
	// ErrMissingObjectID indicates an empty object id.
	ErrMissingObjectID = errors.New("missing object id")
	// ErrObjectNotFound indicates that the requested object is not registered.
	ErrObjectNotFound = errors.New("object not found")
	// ErrObjectExists indicates that an object already exists for the requested identity.
	ErrObjectExists = errors.New("object already exists")
	// ErrMissingAuthenticationHandler indicates that authentication was requested without a handler.
	ErrMissingAuthenticationHandler = errors.New("missing authentication handler")
	// ErrAuthenticationHandlerExists indicates that the runtime already has an authentication handler.
	ErrAuthenticationHandlerExists = errors.New("authentication handler already registered")
	// ErrAuthenticationHandlerUnsupported indicates that an object cannot handle authentication attempts.
	ErrAuthenticationHandlerUnsupported = errors.New("object does not handle authentication attempts")
	// ErrAuthenticationRequired indicates that a request requires prior authentication.
	ErrAuthenticationRequired = errors.New("authentication required")
	// ErrAuthenticatedClientIDInUse indicates that another connection already owns the authenticated id.
	ErrAuthenticatedClientIDInUse = errors.New("authenticated client id already in use")
	// ErrMissingAuthenticationID indicates that authentication succeeded without returning an id.
	ErrMissingAuthenticationID = errors.New("missing authentication id")
	// ErrResponsesUnsupported indicates that the current transport cannot send immediate responses.
	ErrResponsesUnsupported = errors.New("request responses are unsupported")
)

// Runtime owns registered objects and advances the authoritative simulation.
type Runtime struct {
	config                   Config
	tick                     uint64
	objects                  map[string]map[string]Object
	factories                map[string]ObjectFactory
	authenticationHandler    AuthenticationHandler
	features                 Features
	authenticationObjectType string
	authenticationObjectID   string
	mu                       sync.RWMutex
	hooksMu                  sync.Mutex
}

// NewRuntime creates a runtime with normalized configuration.
func NewRuntime(config Config) *Runtime {
	return &Runtime{
		config:    config.Normalized(),
		objects:   map[string]map[string]Object{},
		factories: map[string]ObjectFactory{},
	}
}

// Features returns the runtime's optional feature registry.
func (runtime *Runtime) Features() *Features {
	return &runtime.features
}

// Config returns the normalized runtime configuration.
func (runtime *Runtime) Config() Config {
	return runtime.config
}

// Tick returns the current simulation tick.
func (runtime *Runtime) Tick() uint64 {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	return runtime.tick
}

// RegisterObject adds object to the runtime and runs its initialization hook.
func (runtime *Runtime) RegisterObject(object Object) error {
	objectType, objectID, err := objectIdentity(object)
	if err != nil {
		return err
	}

	runtime.mu.Lock()
	if runtime.objects[objectType] == nil {
		runtime.objects[objectType] = map[string]Object{}
	}
	runtime.objects[objectType][objectID] = object
	runtime.mu.Unlock()

	runtime.features.DisableInterestManagement(objectType, objectID)
	runtime.initializeObject(object, objectType, objectID)

	return nil
}

// RegisterAuthenticationObject registers object as the sole authentication handler.
func (runtime *Runtime) RegisterAuthenticationObject(object Object) error {
	handler, ok := object.(AuthenticationHandler)
	if !ok {
		return ErrAuthenticationHandlerUnsupported
	}

	objectType, objectID, err := objectIdentity(object)
	if err != nil {
		return err
	}

	runtime.mu.Lock()
	if runtime.authenticationHandler != nil {
		runtime.mu.Unlock()
		return fmt.Errorf("%w: %s/%s", ErrAuthenticationHandlerExists, runtime.authenticationObjectType, runtime.authenticationObjectID)
	}

	if runtime.objects[objectType] == nil {
		runtime.objects[objectType] = map[string]Object{}
	}
	runtime.objects[objectType][objectID] = object
	runtime.authenticationHandler = handler
	runtime.authenticationObjectType = objectType
	runtime.authenticationObjectID = objectID
	runtime.mu.Unlock()

	runtime.features.DisableInterestManagement(objectType, objectID)
	runtime.initializeObject(object, objectType, objectID)

	return nil
}

// RemoveObject removes an object and clears related authentication or interest state.
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
	if runtime.authenticationObjectType == objectType && runtime.authenticationObjectID == objectID {
		runtime.authenticationHandler = nil
		runtime.authenticationObjectType = ""
		runtime.authenticationObjectID = ""
	}
	runtime.features.DisableInterestManagement(objectType, objectID)
}

// GetObject returns a registered object by type and id.
func (runtime *Runtime) GetObject(objectType, objectID string) (Object, bool) {
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

// RegisterFactory stores a factory for server-side object creation.
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

// AuthenticationRequired reports whether the runtime has an authentication handler.
func (runtime *Runtime) AuthenticationRequired() bool {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()

	return runtime.authenticationHandler != nil
}

// CreateObject creates and registers an object through a registered factory.
func (runtime *Runtime) CreateObject(objectType, objectID string) (Object, error) {
	objectType = normalizeObjectKey(objectType)
	objectID = normalizeObjectKey(objectID)
	if objectType == "" {
		return nil, ErrMissingObjectType
	}
	if objectID == "" {
		return nil, ErrMissingObjectID
	}
	if _, ok := runtime.GetObject(objectType, objectID); ok {
		return nil, fmt.Errorf("%w: %s/%s", ErrObjectExists, objectType, objectID)
	}

	runtime.mu.RLock()
	factory := runtime.factories[objectType]
	runtime.mu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("%w: %s/%s", ErrObjectNotFound, objectType, objectID)
	}

	object, err := factory.CreateObject(RequestContext{
		ReceivedAt: time.Now(),
		Request: RequestMessage{
			ObjectType: objectType,
			ObjectID:   objectID,
		},
		Runtime:  runtime,
		Features: runtime.Features(),
	})
	if err != nil {
		return nil, err
	}

	createdType, createdID, err := objectIdentity(object)
	if err != nil {
		return nil, err
	}
	if createdType != objectType || createdID != objectID {
		return nil, fmt.Errorf("factory created %s/%s for requested %s/%s", createdType, createdID, objectType, objectID)
	}
	if err := runtime.RegisterObject(object); err != nil {
		return nil, err
	}

	return object, nil
}

// Advance increments the simulation tick and runs object tick hooks.
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
		Features:     runtime.Features(),
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

// HandleRequest routes a client request to the addressed object.
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
	ctx.Features = runtime.Features()

	object, ok := runtime.GetObject(ctx.Request.ObjectType, ctx.Request.ObjectID)
	if !ok {
		return fmt.Errorf("%w: %s/%s", ErrObjectNotFound, ctx.Request.ObjectType, ctx.Request.ObjectID)
	}

	if handler, ok := object.(RequestHandler); ok {
		return handler.OnRequest(ctx)
	}

	return nil
}

// HandleAuthenticationAttempt routes an authentication request to the registered handler.
func (runtime *Runtime) HandleAuthenticationAttempt(ctx AuthenticationContext) (string, error) {
	if ctx.ReceivedAt.IsZero() {
		ctx.ReceivedAt = time.Now()
	}

	runtime.hooksMu.Lock()
	defer runtime.hooksMu.Unlock()

	runtime.mu.RLock()
	ctx.Tick = runtime.tick
	ctx.Runtime = runtime
	ctx.Features = runtime.Features()
	handler := runtime.authenticationHandler
	runtime.mu.RUnlock()
	if handler == nil {
		return "", ErrMissingAuthenticationHandler
	}

	authenticatedID, err := handler.OnAuthenticationAttempt(ctx)
	if err != nil {
		return "", err
	}

	authenticatedID = normalizeObjectKey(authenticatedID)
	if authenticatedID == "" {
		return "", ErrMissingAuthenticationID
	}

	return authenticatedID, nil
}

// Snapshot returns the visible state of all registered objects.
func (runtime *Runtime) Snapshot() SnapshotData {
	runtime.hooksMu.Lock()
	defer runtime.hooksMu.Unlock()

	return snapshotFromObjects(runtime.objectList())
}

// SnapshotForClient returns the visible state for a client after interest filtering.
func (runtime *Runtime) SnapshotForClient(clientID string) SnapshotData {
	clientID = normalizeObjectKey(clientID)

	runtime.hooksMu.Lock()
	defer runtime.hooksMu.Unlock()

	objects := runtime.objectList()
	interest := runtime.features.interestState()
	if !interest.enabled || clientID == "" {
		return snapshotFromObjects(objects)
	}

	playerConfig, ok := interest.playerForClient(clientID)
	if !ok {
		return snapshotFromObjects(objects)
	}

	playerPosition, ok := runtime.interestPositionForObject(objects, playerConfig)
	if !ok {
		return snapshotFromObjects(objects)
	}

	snapshot := SnapshotData{}
	for _, object := range objects {
		if visibility, ok := object.(SnapshotVisibility); ok && !visibility.SnapshotVisible() {
			continue
		}

		objectType, objectID, err := objectIdentity(object)
		if err != nil {
			continue
		}

		objectSnapshot := object.Snapshot()
		if config, ok := interest.objectConfig(objectType, objectID); ok {
			isSelf := objectType == playerConfig.ObjectType && objectID == playerConfig.ObjectID
			if isSelf && !config.IncludeSelf {
				continue
			}

			radius := playerConfig.Radius
			if radius <= 0 {
				radius = config.Radius
			}
			if !isSelf && radius > 0 {
				objectPosition, ok := extractPosition(objectSnapshot, config)
				if ok && !withinInterestRadius(playerPosition, objectPosition, radius) {
					continue
				}
			}
		}

		addSnapshotObject(snapshot, objectType, objectID, objectSnapshot)
	}

	return snapshot
}

// interestPositionForObject returns the configured position for an object in objects.
func (runtime *Runtime) interestPositionForObject(objects []Object, config InterestManagementConfig) (interestPosition, bool) {
	for _, object := range objects {
		objectType, objectID, err := objectIdentity(object)
		if err != nil {
			continue
		}
		if objectType != config.ObjectType || objectID != config.ObjectID {
			continue
		}

		return extractPosition(object.Snapshot(), config)
	}

	return interestPosition{}, false
}

// snapshotFromObjects builds a snapshot from visible objects.
func snapshotFromObjects(objects []Object) SnapshotData {
	snapshot := SnapshotData{}
	for _, object := range objects {
		if visibility, ok := object.(SnapshotVisibility); ok && !visibility.SnapshotVisible() {
			continue
		}

		objectType, objectID, err := objectIdentity(object)
		if err != nil {
			continue
		}

		addSnapshotObject(snapshot, objectType, objectID, object.Snapshot())
	}

	return snapshot
}

// addSnapshotObject inserts a single object snapshot into snapshot.
func addSnapshotObject(snapshot SnapshotData, objectType, objectID string, objectSnapshot any) {
	if snapshot[objectType] == nil {
		snapshot[objectType] = map[string]any{}
	}
	snapshot[objectType][objectID] = objectSnapshot
}

// initializeObject runs the object's initialization hook when present.
func (runtime *Runtime) initializeObject(object Object, objectType, objectID string) {
	if handler, ok := object.(InitHandler); ok {
		handler.OnInit(InitContext{
			object:     object,
			objectType: objectType,
			objectID:   objectID,
			runtime:    runtime,
		})
	}
}

// objectList returns a point-in-time slice of registered objects.
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

// objectIdentity validates and normalizes an object's identity.
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

// normalizeObjectKey trims whitespace from object identity fields.
func normalizeObjectKey(value string) string {
	return strings.TrimSpace(value)
}
