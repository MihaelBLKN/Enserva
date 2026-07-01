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

// main parses command-line flags and starts the configured server.
func main() {
	udpPort := flag.Int("udpPort", 9000, "udp server port")
	udpAddr := flag.String("udpAddr", "", "udp server listen address; overrides udpPort when set")
	tickRate := flag.Int("tickRate", 128, "simulation ticks per second")
	snapshotRate := flag.Int("snapshotRate", 20, "snapshots sent per second")
	deltaSnapshots := flag.Bool("deltaSnapshots", false, "send delta snapshots after each client's initial full snapshot")
	fullSnapshotInterval := flag.Int("fullSnapshotInterval", 64, "maximum emitted snapshots in a delta baseline cycle")
	clientTimeout := flag.Duration("clientTimeout", 5*time.Second, "udp client timeout")
	maxClients := flag.Int("maxClients", 0, "maximum simultaneous UDP clients, 0 allows unlimited clients")
	maxUDPPacketSize := flag.Int("maxUdpPacketSize", 1200, "maximum outbound UDP packet payload size in bytes")
	enableBandwidthBudget := flag.Bool("bandwidthBudget", false, "enable per-client outbound bandwidth budgeting")
	clientBytesPerSecond := flag.Int("clientBytesPerSecond", 0, "outbound byte budget per UDP client per second")
	defaultSnapshotPriority := flag.Int("defaultSnapshotPriority", int(network.OutboundPriorityNormal), "default outbound priority for snapshot objects")
	reliableRetryInterval := flag.Duration("reliableRetryInterval", 100*time.Millisecond, "retry interval for unacknowledged reliable UDP messages")
	reliableMaxAttempts := flag.Int("reliableMaxAttempts", 5, "maximum send attempts for one reliable UDP message")
	reliableQueueLimit := flag.Int("reliableQueueLimit", 64, "maximum queued reliable UDP messages per client")
	maxInputFutureTicks := flag.Uint64("maxInputFutureTicks", 8, "maximum accepted input ticks ahead of the current runtime tick")
	maxInputPastTicks := flag.Uint64("maxInputPastTicks", 2, "maximum accepted input ticks behind the current runtime tick")
	inputBufferLimit := flag.Int("inputBufferLimit", 256, "maximum buffered inputs per client")
	exampleObjects := flag.Bool("exampleObjects", true, "register the sample netObjects package")
	debug := flag.Bool("debug", false, "serve the browser debug interface")
	debugAddr := flag.String("debugAddr", ":9100", "debug web interface address")
	flag.Parse()

	config := network.DefaultConfig()
	config.UDPAddress = udpListenAddress(*udpAddr, *udpPort)
	config.TickRate = *tickRate
	config.SnapshotRate = *snapshotRate
	config.EnableDeltaSnapshots = *deltaSnapshots
	config.FullSnapshotInterval = *fullSnapshotInterval
	config.ClientTimeout = *clientTimeout
	config.MaxClients = *maxClients
	config.MaxUDPPacketSize = *maxUDPPacketSize
	config.EnableBandwidthBudget = *enableBandwidthBudget
	config.ClientBytesPerSecond = *clientBytesPerSecond
	config.DefaultSnapshotPriority = network.OutboundPriority(*defaultSnapshotPriority)
	config.ReliableRetryInterval = *reliableRetryInterval
	config.ReliableMaxAttempts = *reliableMaxAttempts
	config.ReliableQueueLimit = *reliableQueueLimit
	config.MaxInputFutureTicks = *maxInputFutureTicks
	config.MaxInputPastTicks = *maxInputPastTicks
	config.InputBufferLimit = *inputBufferLimit
	config.DebugEnabled = *debug
	config.DebugAddress = *debugAddr

	server := network.NewServer(config)
	if *exampleObjects {
		if err := netobjects.Register(server); err != nil {
			log.Fatal(err)
		}
	}

	log.Fatal(server.ListenAndServe())
}

func udpListenAddress(address string, port int) string {
	address = strings.TrimSpace(address)
	if address != "" {
		return address
	}

	return fmt.Sprintf(":%d", port)
}
