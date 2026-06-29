package network

import (
	"log"
)

type Server struct {
	config  Config
	runtime *Runtime
}

func NewServer(config Config) *Server {
	config = config.Normalized()

	return &Server{
		config:  config,
		runtime: NewRuntime(config),
	}
}

func ListenAndServe(config Config) error {
	return NewServer(config).ListenAndServe()
}

func (server *Server) Config() Config {
	return server.config
}

func (server *Server) Runtime() *Runtime {
	return server.runtime
}

func (server *Server) RegisterObject(object Object) error {
	return server.runtime.RegisterObject(object)
}

func (server *Server) RegisterAuthenticationObject(object Object) error {
	return server.runtime.RegisterAuthenticationObject(object)
}

func (server *Server) RemoveObject(objectType, objectID string) {
	server.runtime.RemoveObject(objectType, objectID)
}

func (server *Server) RegisterFactory(objectType string, factory ObjectFactory) error {
	return server.runtime.RegisterFactory(objectType, factory)
}

func (server *Server) CreateObject(objectType, objectID string) (Object, error) {
	return server.runtime.CreateObject(objectType, objectID)
}

func (server *Server) ListenAndServe() error {
	return server.ListenAndServeUDP()
}

func (server *Server) ListenAndServeUDP() error {
	udpServer := NewUDPServer(server.runtime)

	log.Printf("Enserva UDP API running on %s", server.config.UDPAddress)
	log.Printf("Tick rate: %d/s, snapshots: %d/s", server.config.TickRate, server.config.SnapshotRate)

	return udpServer.ListenAndServe()
}
