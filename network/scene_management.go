package network

// SceneID identifies a server-owned world, map, room, shard, or scene.
type SceneID string

const (
	// SceneGlobal marks an object as visible to clients in every scene.
	SceneGlobal SceneID = "*"
)

// SceneManager stores server-owned scene membership for clients and objects.
type SceneManager struct {
	enabled bool
	clients map[string]SceneID
	objects map[string]SceneID
}

type sceneState struct {
	enabled bool
	clients map[string]SceneID
	objects map[string]SceneID
}

// Scenes returns the manager, creating it when features is non-nil.
func (features *Features) Scenes() *SceneManager {
	if features == nil {
		return nil
	}

	features.mu.Lock()
	defer features.mu.Unlock()

	if features.scenes == nil {
		features.scenes = newSceneManager()
	}

	return features.scenes
}

// SetObjectScene assigns an object to a scene. Unassigned objects are visible everywhere.
func (features *Features) SetObjectScene(objectType, objectID string, sceneID SceneID) bool {
	if features == nil {
		return false
	}

	objectType = normalizeObjectKey(objectType)
	objectID = normalizeObjectKey(objectID)
	sceneID = normalizeSceneID(sceneID)
	if objectType == "" || objectID == "" || sceneID == "" {
		return false
	}

	features.mu.Lock()
	defer features.mu.Unlock()

	if features.scenes == nil {
		features.scenes = newSceneManager()
	}

	features.scenes.objects[interestObjectKey(objectType, objectID)] = sceneID
	features.scenes.enabled = len(features.scenes.clients) > 0
	return true
}

// SetObjectSceneForObject assigns object to a scene.
func (features *Features) SetObjectSceneForObject(object Object, sceneID SceneID) bool {
	objectType, objectID, err := objectIdentity(object)
	if err != nil {
		return false
	}

	return features.SetObjectScene(objectType, objectID, sceneID)
}

// SetObjectGlobal marks an object as visible to clients in every scene.
func (features *Features) SetObjectGlobal(objectType, objectID string) bool {
	return features.SetObjectScene(objectType, objectID, SceneGlobal)
}

// ClearObjectScene removes explicit scene membership from an object.
func (features *Features) ClearObjectScene(objectType, objectID string) {
	if features == nil {
		return
	}

	objectType = normalizeObjectKey(objectType)
	objectID = normalizeObjectKey(objectID)
	if objectType == "" || objectID == "" {
		return
	}

	features.mu.Lock()
	defer features.mu.Unlock()

	if features.scenes == nil {
		return
	}

	delete(features.scenes.objects, interestObjectKey(objectType, objectID))
	features.scenes.enabled = len(features.scenes.clients) > 0
}

// ObjectScene returns the explicit scene assigned to an object.
func (features *Features) ObjectScene(objectType, objectID string) (SceneID, bool) {
	if features == nil {
		return "", false
	}

	objectType = normalizeObjectKey(objectType)
	objectID = normalizeObjectKey(objectID)
	if objectType == "" || objectID == "" {
		return "", false
	}

	features.mu.RLock()
	defer features.mu.RUnlock()

	if features.scenes == nil {
		return "", false
	}

	sceneID, ok := features.scenes.objects[interestObjectKey(objectType, objectID)]
	return sceneID, ok
}

// SetClientScene assigns a client to a scene used for snapshot filtering.
func (features *Features) SetClientScene(clientID string, sceneID SceneID) bool {
	if features == nil {
		return false
	}

	clientID = normalizeObjectKey(clientID)
	sceneID = normalizeSceneID(sceneID)
	if clientID == "" || sceneID == "" {
		return false
	}

	features.mu.Lock()
	defer features.mu.Unlock()

	if features.scenes == nil {
		features.scenes = newSceneManager()
	}

	features.scenes.clients[clientID] = sceneID
	features.scenes.enabled = len(features.scenes.clients) > 0
	return true
}

// ClearClientScene removes explicit scene membership from a client.
func (features *Features) ClearClientScene(clientID string) {
	if features == nil {
		return
	}

	clientID = normalizeObjectKey(clientID)
	if clientID == "" {
		return
	}

	features.mu.Lock()
	defer features.mu.Unlock()

	if features.scenes == nil {
		return
	}

	delete(features.scenes.clients, clientID)
	features.scenes.enabled = len(features.scenes.clients) > 0
}

// ClientScene returns the scene currently assigned to clientID.
func (features *Features) ClientScene(clientID string) (SceneID, bool) {
	if features == nil {
		return "", false
	}

	clientID = normalizeObjectKey(clientID)
	if clientID == "" {
		return "", false
	}

	features.mu.RLock()
	defer features.mu.RUnlock()

	if features.scenes == nil {
		return "", false
	}

	sceneID, ok := features.scenes.clients[clientID]
	return sceneID, ok
}

// DisableSceneManagement removes scene state for an object being unregistered.
func (features *Features) DisableSceneManagement(objectType, objectID string) {
	features.ClearObjectScene(objectType, objectID)
}

func newSceneManager() *SceneManager {
	return &SceneManager{
		clients: map[string]SceneID{},
		objects: map[string]SceneID{},
	}
}

func (features *Features) sceneState() sceneState {
	if features == nil {
		return sceneState{}
	}

	features.mu.RLock()
	defer features.mu.RUnlock()

	if features.scenes == nil || !features.scenes.enabled {
		return sceneState{}
	}

	state := sceneState{
		enabled: features.scenes.enabled,
		clients: make(map[string]SceneID, len(features.scenes.clients)),
		objects: make(map[string]SceneID, len(features.scenes.objects)),
	}
	for clientID, sceneID := range features.scenes.clients {
		state.clients[clientID] = sceneID
	}
	for key, sceneID := range features.scenes.objects {
		state.objects[key] = sceneID
	}

	return state
}

func (state sceneState) clientScene(clientID string) (SceneID, bool) {
	if !state.enabled {
		return "", false
	}

	sceneID, ok := state.clients[normalizeObjectKey(clientID)]
	return sceneID, ok
}

func (state sceneState) objectVisibleToClient(clientScene SceneID, filtersScene bool, objectType, objectID string) bool {
	if !state.enabled || !filtersScene {
		return true
	}

	objectScene, ok := state.objects[interestObjectKey(objectType, objectID)]
	if !ok || objectScene == SceneGlobal {
		return true
	}

	return objectScene == clientScene
}

func normalizeSceneID(sceneID SceneID) SceneID {
	return SceneID(normalizeObjectKey(string(sceneID)))
}
