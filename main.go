package main

import (
	netobjects "Enserva/netObjects"
	"Enserva/network"
	"flag"
	"fmt"
	"log"
	"time"
)

func main() {
	udpPort := flag.Int("udpPort", 9000, "udp server port")
	tickRate := flag.Int("tickRate", 128, "simulation ticks per second")
	snapshotRate := flag.Int("snapshotRate", 20, "snapshots sent per second")
	clientTimeout := flag.Duration("clientTimeout", 5*time.Second, "udp client timeout")
	exampleObjects := flag.Bool("exampleObjects", true, "register the sample netObjects package")
	flag.Parse()

	config := network.DefaultConfig()
	config.UDPAddress = fmt.Sprintf(":%d", *udpPort)
	config.TickRate = *tickRate
	config.SnapshotRate = *snapshotRate
	config.ClientTimeout = *clientTimeout

	server := network.NewServer(config)
	if *exampleObjects {
		if err := netobjects.Register(server); err != nil {
			log.Fatal(err)
		}
	}

	log.Fatal(server.ListenAndServe())
}
