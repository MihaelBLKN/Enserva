use std::env;
use std::time::Duration;

use enserva_rust_client_example::{EnservaUdpClient, ServerMessage};

fn main() -> Result<(), Box<dyn std::error::Error>> {
    let args: Vec<String> = env::args().collect();
    let server_addr = args.get(1).map(String::as_str).unwrap_or("127.0.0.1:9000");
    let client_name = args.get(2).map(String::as_str).unwrap_or("rust-client");
    let token = args.get(3).map(String::as_str).unwrap_or("dev-token");

    let mut client = EnservaUdpClient::connect(server_addr, client_name, token)?;
    client.set_read_timeout(Some(Duration::from_millis(250)))?;

    client.send_player_input("player-1", 1, 120, 1.0, 0.0, 0.0)?;
    client.send_ping(42)?;

    for _ in 0..20 {
        client.update()?;

        match client.receive_one()? {
            Some(ServerMessage::Welcome(welcome)) => {
                println!(
                    "welcome client_id={} authenticated_id={} protocol={} capabilities=0x{:x} max_packet_size={}",
                    welcome.client_id,
                    welcome.authenticated_id,
                    welcome.protocol_version,
                    welcome.capabilities.bits(),
                    welcome.max_packet_size
                );
            }
            Some(ServerMessage::Pong(pong)) => println!("pong nonce={}", pong.nonce),
            Some(ServerMessage::Error(error)) => {
                eprintln!("server error {}: {}", error.code, error.message);
            }
            Some(ServerMessage::Disconnect(disconnect)) => {
                eprintln!(
                    "server disconnect {}: {}",
                    disconnect.code, disconnect.message
                );
                break;
            }
            Some(ServerMessage::WorldSnapshot(snapshot)) => {
                println!(
                    "snapshot tick={} object_types={}",
                    snapshot.tick,
                    snapshot.objects.len()
                );
            }
            Some(ServerMessage::WorldDeltaSnapshot(delta)) => {
                println!(
                    "delta tick={} spawned_types={} changed_types={} despawned={}",
                    delta.tick,
                    delta.spawned.len(),
                    delta.changed.len(),
                    delta.despawned.len()
                );
            }
            Some(ServerMessage::Custom(message)) => {
                println!(
                    "custom message type=0x{:04x} bytes={}",
                    message.message_type.as_u16(),
                    message.payload.len()
                );
            }
            None => {}
        }
    }

    Ok(())
}
