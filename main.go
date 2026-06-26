package main

import (
	"Enserva/network"
	"Enserva/objects"
	"flag"
	"fmt"
	"log"
)

func main() {
	networkProtocol := flag.String("networkProtocol", "ws", "network protocol to use (ws or udp)")
	gameWorldX := flag.Float64("gameWorldX", 720, "game world width")
	gameWorldY := flag.Float64("gameWorldY", 405, "game world height")
	gameWorldZ := flag.Float64("gameWorldZ", objects.DefaultWorldZ, "game world height/depth for 3d mode")
	playerDimension := flag.String("playerDimension", "2d", "player dimension mode (2d or 3d)")
	httpPort := flag.Int("httpPort", 8080, "debug client http port")
	udpPort := flag.Int("udpPort", 9000, "udp server port")
	flag.Parse()

	dimension, err := objects.ParseDimension(*playerDimension)
	if err != nil {
		log.Fatal(err)
	}

	world := objects.NewWorldConfig(*gameWorldX, *gameWorldY, *gameWorldZ, dimension)
	httpAddress := fmt.Sprintf(":%d", *httpPort)
	udpAddress := fmt.Sprintf(":%d", *udpPort)

	switch *networkProtocol {
	case "ws":
		log.Fatal(network.ServeWebSocketDebugClientWithAddress(world, httpAddress))
	case "udp":
		log.Fatal(network.ServeUDPDebugClientWithAddresses(world, httpAddress, udpAddress))
	default:
		log.Fatalf("Invalid network protocol: %s", *networkProtocol)
	}
}
