package network

import (
	"log"
	"sync"
)

// Server wires the runtime to configured network transports.
type Server struct {
	config  Config
	runtime *Runtime
	mu      sync.RWMutex
	udp     *UDPServer
}

// NewServer creates a server with a new runtime.
func NewServer(config Config) *Server {
	config = config.Normalized()

	return &Server{
		config:  config,
		runtime: NewRuntime(config),
	}
}

// ListenAndServe creates a server and starts its default transport.
func ListenAndServe(config Config) error {
	return NewServer(config).ListenAndServe()
}

// Config returns the normalized server configuration.
func (server *Server) Config() Config {
	return server.config
}

// Runtime returns the server's authoritative runtime.
func (server *Server) Runtime() *Runtime {
	return server.runtime
}

// UDPServer returns the active UDP server when one has been started.
func (server *Server) UDPServer() *UDPServer {
	server.mu.RLock()
	defer server.mu.RUnlock()

	return server.udp
}

// RegisterObject adds object to the server runtime.
func (server *Server) RegisterObject(object Object) error {
	return server.runtime.RegisterObject(object)
}

// RegisterAuthenticationObject adds object as the server's authentication handler.
func (server *Server) RegisterAuthenticationObject(object Object) error {
	return server.runtime.RegisterAuthenticationObject(object)
}

// RemoveObject removes an object from the server runtime.
func (server *Server) RemoveObject(objectType, objectID string) {
	server.runtime.RemoveObject(objectType, objectID)
}

// RegisterFactory stores a factory for server-side object creation.
func (server *Server) RegisterFactory(objectType string, factory ObjectFactory) error {
	return server.runtime.RegisterFactory(objectType, factory)
}

// RegisterWireMessage adds a custom binary protocol message definition.
func (server *Server) RegisterWireMessage(definition WireMessageDefinition) error {
	return server.runtime.RegisterWireMessage(definition)
}

// CreateObject creates and registers an object through a registered factory.
func (server *Server) CreateObject(objectType, objectID string) (Object, error) {
	return server.runtime.CreateObject(objectType, objectID)
}

// ListenAndServe starts the server's default transport.
func (server *Server) ListenAndServe() error {
	return server.ListenAndServeUDP()
}

// ListenAndServeUDP starts the UDP transport and optional debug interface.
func (server *Server) ListenAndServeUDP() error {
	udpServer := NewUDPServer(server.runtime)
	server.mu.Lock()
	server.udp = udpServer
	server.mu.Unlock()

	if server.config.DebugEnabled {
		go func() {
			log.Printf("Enserva debug UI running on %s", debugHTTPURL(server.config.DebugAddress))
			if err := server.ListenAndServeDebug(); err != nil {
				log.Printf("debug UI stopped: %v", err)
			}
		}()
	}

	log.Printf("Enserva UDP API running on %s", server.config.UDPAddress)
	log.Printf("Tick rate: %d/s, snapshots: %d/s", server.config.TickRate, server.config.SnapshotRate)

	return udpServer.ListenAndServe()
}
