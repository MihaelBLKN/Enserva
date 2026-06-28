package netobjects

import "Enserva/network"

func Register(server *network.Server) error {
	if err := server.RegisterFactory("player", network.ObjectFactoryFunc(PlayerFactory)); err != nil {
		return err
	}
	if err := server.RegisterFactory("building", network.ObjectFactoryFunc(BuildingFactory)); err != nil {
		return err
	}

	return nil
}
