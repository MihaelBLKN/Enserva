package main

import (
	netobjects "Enserva/netObjects"
	"Enserva/network"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"
)

func main() {
	networkProtocol := flag.String("networkProtocol", "ws", "network protocol to use (ws or udp)")
	httpPort := flag.Int("httpPort", 8080, "http port for websocket or udp bridge")
	udpPort := flag.Int("udpPort", 9000, "udp server port")
	tickRate := flag.Int("tickRate", 128, "simulation ticks per second")
	snapshotRate := flag.Int("snapshotRate", 20, "snapshots sent per second")
	clientTimeout := flag.Duration("clientTimeout", 5*time.Second, "udp client timeout")
	staticDir := flag.String("staticDir", "", "optional directory to serve over http")
	exampleObjects := flag.Bool("exampleObjects", true, "register the sample netObjects package")
	flag.Parse()

	config := network.DefaultConfig()
	config.Protocol = network.Protocol(strings.ToLower(strings.TrimSpace(*networkProtocol)))
	config.HTTPAddress = fmt.Sprintf(":%d", *httpPort)
	config.UDPAddress = fmt.Sprintf(":%d", *udpPort)
	config.TickRate = *tickRate
	config.SnapshotRate = *snapshotRate
	config.ClientTimeout = *clientTimeout
	config.StaticDir = strings.TrimSpace(*staticDir)

	server := network.NewServer(config)
	if *exampleObjects {
		if err := netobjects.Register(server); err != nil {
			log.Fatal(err)
		}
	}

	log.Fatal(server.ListenAndServe())
}
