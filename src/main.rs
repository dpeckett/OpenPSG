#![cfg_attr(not(test), no_std)]
#![cfg_attr(not(test), no_main)]

use crate::cs1237_spi::{Config as Cs1237Config, Cs1237, SamplesPerSecond};
use crate::embassy_sntpc::{set_time, TimestampGen};
use crate::net_util::generate_mac_address;
use core::net::{IpAddr, SocketAddr};
use core::option::Option::*;
use core::result::Result::*;
use defmt::{error, info, warn};
use embassy_executor::Spawner;
use embassy_net::tcp::{Error as TcpReadError, TcpSocket};
use embassy_net::udp::{PacketMetadata, UdpSocket};
use embassy_net::{Stack, StackResources};
use embassy_stm32::eth::generic_smi::GenericSMI;
use embassy_stm32::eth::{Ethernet, PacketQueue};
use embassy_stm32::peripherals::ETH;
use embassy_stm32::rng::Rng;
use embassy_stm32::time::Hertz;
use embassy_stm32::{bind_interrupts, eth, peripherals, rng, Config};
use embassy_sync::{blocking_mutex::raw::ThreadModeRawMutex, mutex::Mutex};
use embassy_time::{Duration, Timer};
use embedded_jsonrpc::{RpcRequest, RpcServer, JSONRPC_VERSION};
use heapless::Vec;
use rand_core::RngCore;
use static_cell::StaticCell;
use {defmt_rtt as _, panic_probe as _};

mod cs1237_spi;
mod embassy_sntpc;
mod net_util;

bind_interrupts!(struct Irqs {
    ETH => eth::InterruptHandler;
    RNG => rng::InterruptHandler<peripherals::RNG>;
});

type Device = Ethernet<'static, ETH, GenericSMI>;

const NTP_PORT: u16 = 123;
const NTP_PACKET_SIZE: usize = 48;

#[embassy_executor::task]
async fn net_task(mut runner: embassy_net::Runner<'static, Device>) -> ! {
    runner.run().await
}

#[embassy_executor::task]
async fn timesync_task(stack: Stack<'static>) -> ! {
    let timestamp_gen = TimestampGen::default();
    let context = sntpc::NtpContext::new(timestamp_gen);

    // Create UDP socket with 2 NTP packets worth of buffer space.
    let mut rx_meta = [PacketMetadata::EMPTY; 1];
    let mut rx_buffer = [0; NTP_PACKET_SIZE];
    let mut tx_meta = [PacketMetadata::EMPTY; 1];
    let mut tx_buffer = [0; NTP_PACKET_SIZE];

    let mut socket = UdpSocket::new(
        stack,
        &mut rx_meta,
        &mut rx_buffer,
        &mut tx_meta,
        &mut tx_buffer,
    );
    socket.bind(NTP_PORT).unwrap();

    // Get the address of the network gateway.
    let gateway = stack.config_v4().unwrap().gateway;
    if gateway.is_none() {
        panic!("Expected a gateway address");
    }

    loop {
        let addr = SocketAddr::new(IpAddr::V4(gateway.unwrap()), NTP_PORT);

        match sntpc::get_time(addr, &socket, context).await {
            Ok(time) => {
                set_time(time);

                info!("Time synchronized: {:?}", time);
            }
            Err(err) => {
                error!("Time synchronization error: {:?}", err);
            }
        }

        Timer::after(Duration::from_secs(30)).await;
    }
}

/*
const DEFAULT_ADC_CHUNK_SIZE: usize = 32;
const MAX_ADC_CHUNK_SIZE: usize = 256;

#[embassy_executor::task]
async fn adc_sampling_task(
    rpc_server: &'static RpcServer<'static, TcpReadError>,
    adc: &'static Mutex<
        ThreadModeRawMutex,
        Cs1237<
            'static,
            peripherals::SPI1,
            peripherals::DMA2_CH3,
            peripherals::DMA2_CH2,
            peripherals::PA6,
        >,
    >,
    sampling_enabled: &'static Mutex<ThreadModeRawMutex, bool>,
    chunk_size: &'static Mutex<ThreadModeRawMutex, usize>,
) {
    let mut samples = Vec::<i32, MAX_ADC_CHUNK_SIZE>::new();
    loop {
        {
            let sampling_enabled = { *sampling_enabled.lock().await };

            if !sampling_enabled {
                embassy_time::Timer::after(embassy_time::Duration::from_millis(100)).await;
                continue;
            }
        }

        samples.clear();

        let size = *chunk_size.lock().await; // Get the current chunk size

        {
            let mut adc = adc.lock().await;

            for _ in 0..size {
                match adc.read().await {
                    Ok(value) => samples.push(value).unwrap_or(()),
                    Err(e) => {
                        warn!("ADC read error: {:?}", e);
                        samples.push(0).unwrap_or(());
                    }
                }
            }
        }

        let notification: RpcRequest<'_, &[i32]> = RpcRequest {
            jsonrpc: JSONRPC_VERSION,
            id: None,
            method: "adc.samples",
            params: Some(&samples),
        };

        let mut notification_json = [0u8; 512];
        let notification_len =
            serde_json_core::to_slice(&notification, &mut notification_json).unwrap();

        rpc_server
            .notify(&notification_json[..notification_len])
            .await
            .unwrap();
    }
}*/

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

    // Initialize peripherals
    /*
    static ADC: StaticCell<
        Mutex<
            ThreadModeRawMutex,
            Cs1237<
                'static,
                peripherals::SPI1,
                peripherals::DMA2_CH3,
                peripherals::DMA2_CH2,
                peripherals::PA6,
            >,
        >,
    > = StaticCell::new();

    let mut adc_config = Cs1237Config::default();
    adc_config.sample_rate = SamplesPerSecond::SPS640;

    let adc = &*ADC.init(Mutex::new(
        Cs1237::try_new(
            p.SPI1, p.PA5, p.PA6, p.DMA2_CH3, p.DMA2_CH2, p.EXTI6, adc_config,
        )
        .await
        .unwrap(),
    ));

    static SAMPLING_ENABLED: StaticCell<Mutex<ThreadModeRawMutex, bool>> = StaticCell::new();
    let sampling_enabled = SAMPLING_ENABLED.init(Mutex::new(true));

    static CHUNK_SIZE: StaticCell<Mutex<ThreadModeRawMutex, usize>> = StaticCell::new();
    let chunk_size = CHUNK_SIZE.init(Mutex::new(DEFAULT_ADC_CHUNK_SIZE));
*/

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
    let config = embassy_net::Config::dhcpv4(embassy_net::DhcpConfig::default());

    // Init network stack
    static RESOURCES: StaticCell<StackResources<3>> = StaticCell::new();
    let (stack, runner) =
        embassy_net::new(device, config, RESOURCES.init(StackResources::new()), seed);

    // Launch network task
    spawner.spawn(net_task(runner)).unwrap();

    // Ensure DHCP configuration is up before trying connect
    stack.wait_config_up().await;

    // Begin synchronizing time with SNTP server
    spawner.spawn(timesync_task(stack.clone())).unwrap();

    // Get the IP address that was assigned to us
    let ip_addr = stack.config_v4().unwrap().address;

    // Create JSON-RPC server
    static RPC_SERVER: StaticCell<RpcServer<'static, TcpReadError>> = StaticCell::new();
    let rpc_server = RPC_SERVER.init_with(RpcServer::new);

    // Register handlers
    /*
        static ADC_CONFIGURE_HANDLER: StaticCell<AdcHandler> = StaticCell::new();
        let adc_configure_handler = {
            let adc = &*adc;
            let sampling_enabled = &*sampling_enabled;
            let chunk_size = &*chunk_size;
            ADC_CONFIGURE_HANDLER.init_with(|| AdcHandler { adc, sampling_enabled, chunk_size })
        };

        rpc_server
            .register_method("adc.configure", adc_configure_handler)
            .unwrap();
    */

    // Launch ADC reading task.
    /*  spawner.spawn(adc_sampling_task(
        rpc_server,
        adc,
        sampling_enabled,
        chunk_size
    )).unwrap();*/

    // Then we can use it!
    let mut rx_buffer = [0; 4096];
    let mut tx_buffer = [0; 4096];

    loop {
        let mut socket = TcpSocket::new(stack, &mut rx_buffer, &mut tx_buffer);
        socket.set_timeout(Some(embassy_time::Duration::from_secs(10)));
        socket.set_keep_alive(Some(embassy_time::Duration::from_secs(5)));

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
