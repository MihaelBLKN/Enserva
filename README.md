# Enserva - Learning project and solution for my own projects

This is an game server, shortened. It works depending on how you feel. Still work in progress but oh well! It supports both WebSocket, UDP WebSocket and pure UDP connections. So, both browser and outside of browser! Summarized for idiots.

Remember, still work in progress so a lot of features for making it easier for developers to use and configure are still missing.

## FLAGS

- -networkProtocol (ws, udp)
- -playerDimension (2d, 3d)
- -gameWorldX
- -gameWorldY
- -gameWorldZ
- -httpPort (this is where the debug web page runs)
- -udpPort (this is where the actual udp server runs)

## FEATURES

- Reconciliation
- Interpolation
- Server-side authority
- World bounds enforcement
- Obstacle enforcement
- UDP connection
- WebSocket connection

## EXAMPLE

go run . -networkProtocol udp -playerDimension 3d
