package network

import (
	"fmt"
	"log"
	"net/http"
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

func (server *Server) RemoveObject(objectType, objectID string) {
	server.runtime.RemoveObject(objectType, objectID)
}

func (server *Server) RegisterFactory(objectType string, factory ObjectFactory) error {
	return server.runtime.RegisterFactory(objectType, factory)
}

func (server *Server) ListenAndServe() error {
	switch server.config.Protocol {
	case ProtocolWebSocket:
		return server.ListenAndServeWebSocket()
	case ProtocolUDP:
		return server.ListenAndServeUDP()
	default:
		return fmt.Errorf("invalid network protocol: %s", server.config.Protocol)
	}
}

func (server *Server) ListenAndServeWebSocket() error {
	webSocketServer := NewWebSocketServer(server.runtime)
	go webSocketServer.runTickLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", webSocketServer.Handle)
	server.handleStaticOrStatus(mux)

	log.Printf("Enserva WebSocket API running on %s", server.config.HTTPAddress)
	log.Printf("Tick rate: %d/s, snapshots: %d/s", server.config.TickRate, server.config.SnapshotRate)

	return http.ListenAndServe(server.config.HTTPAddress, mux)
}

func (server *Server) ListenAndServeUDP() error {
	udpServer := NewUDPServer(server.runtime)

	go func() {
		if err := udpServer.ListenAndServe(); err != nil {
			log.Fatal(err)
		}
	}()

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", handleUDPBridge(server.config.UDPAddress))
	mux.HandleFunc("/udp-bridge", handleUDPBridge(server.config.UDPAddress))
	server.handleStaticOrStatus(mux)

	log.Printf("Enserva UDP API running on %s", server.config.UDPAddress)
	log.Printf("Enserva UDP bridge running on %s", server.config.HTTPAddress)
	log.Printf("Tick rate: %d/s, snapshots: %d/s", server.config.TickRate, server.config.SnapshotRate)

	return http.ListenAndServe(server.config.HTTPAddress, mux)
}

func (server *Server) handleStaticOrStatus(mux *http.ServeMux) {
	if server.config.StaticDir != "" {
		mux.Handle("/", serveDebugFiles(server.config.StaticDir))
		return
	}

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Enserva API server is running.\n"))
	})
}
