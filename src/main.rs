#![no_std]
#![no_main]

use core::convert::TryFrom;
use core::option::Option::*;
use core::result::Result::*;
use defmt::*;
use embassy_executor::Spawner;
use embassy_net::tcp::TcpSocket;
use embassy_net::{Stack, StackResources};
use embassy_stm32::eth::generic_smi::GenericSMI;
use embassy_stm32::eth::{Ethernet, PacketQueue};
use embassy_stm32::peripherals::ETH;
use embassy_stm32::rng::Rng;
use embassy_stm32::time::Hertz;
use embassy_stm32::uid::{uid, uid_hex};
use embassy_stm32::{bind_interrupts, eth, peripherals, rng, Config};
use embedded_jsonrpc::{RpcError, RpcErrorCode, RpcServer, JSONRPC_VERSION};
use rand_core::RngCore;
use serde::{self, Deserialize, Serialize};
use heapless::String;
use static_cell::StaticCell;
use {defmt_rtt as _, panic_probe as _};

bind_interrupts!(struct Irqs {
    ETH => eth::InterruptHandler;
    RNG => rng::InterruptHandler<peripherals::RNG>;
});

type Device = Ethernet<'static, ETH, GenericSMI>;

#[embassy_executor::task]
async fn net_task(stack: &'static Stack<Device>) -> ! {
    stack.run().await
}

#[embassy_executor::main]
async fn main(spawner: Spawner) -> ! {
    let mut config = Config::default();
    {
        use embassy_stm32::rcc::*;
        config.rcc.hse = Some(Hse {
            freq: Hertz(8_000_000),
            mode: HseMode::Bypass,
        });
        config.rcc.pll_src = PllSource::HSE;
        config.rcc.pll = Some(Pll {
            prediv: PllPreDiv::DIV4,
            mul: PllMul::MUL216,
            divp: Some(PllPDiv::DIV2), // 8mhz / 4 * 216 / 2 = 216Mhz
            divq: None,
            divr: None,
        });
        config.rcc.ahb_pre = AHBPrescaler::DIV1;
        config.rcc.apb1_pre = APBPrescaler::DIV4;
        config.rcc.apb2_pre = APBPrescaler::DIV2;
        config.rcc.sys = Sysclk::PLL1_P;
    }
    let p = embassy_stm32::init(config);

    // Generate random seed.
    let mut rng = Rng::new(p.RNG, Irqs);
    let mut seed = [0; 8];
    rng.fill_bytes(&mut seed);
    let seed = u64::from_le_bytes(seed);

    static PACKETS: StaticCell<PacketQueue<16, 16>> = StaticCell::new();
    let device = Ethernet::new(
        PACKETS.init(PacketQueue::<16, 16>::new()),
        p.ETH,
        Irqs,
        p.PA1,
        p.PA2,
        p.PC1,
        p.PA7,
        p.PC4,
        p.PC5,
        p.PG13,
        p.PB13,
        p.PG11,
        GenericSMI::new(0),
        generate_mac_address(),
    );

    // Acquire network configuration using DHCP.
    let mut dhcp_config = embassy_net::DhcpConfig::default();
    dhcp_config.hostname = Some(String::try_from(uid_hex()).unwrap());
    let config = embassy_net::Config::dhcpv4(dhcp_config);

    // Init network stack
    static STACK: StaticCell<Stack<Device>> = StaticCell::new();
    static RESOURCES: StaticCell<StackResources<2>> = StaticCell::new();
    let stack = &*STACK.init(Stack::new(
        device,
        config,
        RESOURCES.init(StackResources::<2>::new()),
        seed,
    ));

    // Launch network task
    unwrap!(spawner.spawn(net_task(stack)));

    // Ensure DHCP configuration is up before trying connect
    stack.wait_config_up().await;

    // Get the IP address that was assigned to us
    let ip_addr = stack.config_v4().unwrap().address;

    info!("Network task initialized");

    // Create JSON-RPC server
    let mut rpc_server: RpcServer<2, 256> = RpcServer::new();

    // Register methods
    rpc_server.register_method("add", add_handler);

    // Then we can use it!
    let mut rx_buffer = [0; 4096];
    let mut tx_buffer = [0; 4096];

    loop {
        let mut socket = TcpSocket::new(stack, &mut rx_buffer, &mut tx_buffer);
        socket.set_timeout(Some(embassy_time::Duration::from_secs(10)));

        info!("Listening on {:?}:1234", ip_addr);

        if let Err(e) = socket.accept(1234).await {
            warn!("accept error: {:?}", e);
            continue;
        }

        info!(
            "Received connection from {:?}",
            socket.remote_endpoint().unwrap()
        );

        // Serve the JSON-RPC requests on the socket
        if let Err(e) = rpc_server.serve(&mut socket).await {
            warn!("JSON-RPC error: {:?}", e);
        }

        info!("Connection closed");
    }
}

fn generate_mac_address() -> [u8; 6] {
    let mut hasher = adler::Adler32::new();

    // Form the basis of our OUI octets
    let bin_name = env!("CARGO_BIN_NAME").as_bytes();
    hasher.write_slice(bin_name);
    let oui = hasher.checksum().to_ne_bytes();

    // Form the basis of our NIC octets.
    hasher.write_slice(uid());
    let nic = hasher.checksum().to_ne_bytes();

    // To make it adhere to EUI-48, we set it to be a unicast locally administered
    // address
    [
        oui[0] & 0b1111_1100 | 0b0000_0010,
        oui[1],
        oui[2],
        nic[0],
        nic[1],
        nic[2],
    ]
}

/// Example custom request type
#[derive(Deserialize)]
struct AddRequest {
    params: [i32; 2],
}

/// Example custom response type
#[derive(Serialize)]
struct AddResponse<'a> {
    jsonrpc: &'a str,
    id: Option<u64>,
    result: i32,
}

fn add_handler(
    id: Option<u64>,
    request_json: &[u8],
    response_json: &mut [u8],
) -> Result<usize, RpcError<'static>> {
    let request: AddRequest = match serde_json_core::from_slice(request_json) {
        Ok((request, _remainder)) => request,
        Err(_) => {
            warn!("Error parsing JSON-RPC add request");
            return Err(RpcError::from_code(RpcErrorCode::ParseError));
        }
    };

    let result: i32 = request.params[0] + request.params[1];

    let response = AddResponse {
        jsonrpc: JSONRPC_VERSION,
        id,
        result,
    };

    Ok(serde_json_core::to_slice(&response, &mut response_json[..]).unwrap())
}
