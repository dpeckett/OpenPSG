use byteorder::{BigEndian, ByteOrder};
use defmt::*;
use embassy_stm32::exti::{Channel as ExtiChannel, ExtiInput};
use embassy_stm32::gpio::{Flex, Input, Level, Output, Pull, Speed};
use embassy_stm32::spi::{Config as SpiConfig, Instance, MisoPin, RxDma, SckPin, Spi, TxDma};
use embassy_stm32::Peripheral;
use embassy_time::{with_timeout, Duration, Timer};

/// Sampling rates for the CS1237 ADC.
#[derive(Clone, Copy, Debug)]
#[allow(unused)]
pub enum SamplesPerSecond {
    SPS10 = 0,
    SPS40 = 1,
    SPS640 = 2,
    SPS1280 = 3,
}

/// Gain settings for the CS1237 ADC.
#[derive(Clone, Copy, Debug)]
#[allow(unused)]
pub enum Gain {
    G1 = 0,
    G2 = 1,
    G64 = 2,
    G128 = 3,
}

/// Selectable channels on the CS1237 ADC.
#[derive(Clone, Copy, Debug)]
#[allow(unused)]
pub enum Channel {
    ChannelA = 0,
    Reserved = 1,
    Temperature = 2,
    InternalShort = 3,
}

/// Configuration parameters for the CS1237 ADC.
#[derive(Clone, Copy, Debug)]
pub struct Config {
    pub sample_rate: SamplesPerSecond,
    pub gain: Gain,
    pub channel: Channel,
}

impl Default for Config {
    fn default() -> Self {
        Self {
            sample_rate: SamplesPerSecond::SPS10,
            gain: Gain::G128,
            channel: Channel::ChannelA,
        }
    }
}

/// Errors that can occur when interacting with the CS1237 ADC.
#[derive(PartialEq, Eq, Clone, Copy, Debug, defmt::Format)]
pub enum Error {
    SpiError,
    Timeout,
}

/// CS1237 ADC interface.
pub struct Cs1237<
    'd,
    SpiInstance: Instance,
    Tx: TxDma<SpiInstance>,
    Rx: RxDma<SpiInstance>,
    DataPin: MisoPin<SpiInstance>,
> {
    spi_dev: Spi<'d, SpiInstance, Tx, Rx>,
    drdy_pin: ExtiInput<'d, DataPin>,
}

impl<
        'd,
        SpiInstance: Instance,
        Tx: TxDma<SpiInstance>,
        Rx: RxDma<SpiInstance>,
        DataPin: MisoPin<SpiInstance>,
    > Cs1237<'d, SpiInstance, Tx, Rx, DataPin>
{
    /// Initializes a new CS1237 ADC interface.
    pub async fn try_new(
        spi: impl Peripheral<P = SpiInstance> + 'd,
        clk: impl Peripheral<P = impl SckPin<SpiInstance>> + 'd,
        data: DataPin,
        txdma: impl Peripheral<P = Tx> + 'd,
        rxdma: impl Peripheral<P = Rx> + 'd,
        interrupt_channel: impl ExtiChannel + Peripheral<P = DataPin::ExtiChannel> + 'd,
        config: Config,
    ) -> Result<Self, Error> {
        let mut drdy_pin = ExtiInput::new(
            Input::new(unsafe { Peripheral::clone_unchecked(&data) }, Pull::None),
            interrupt_channel,
        );

        {
            let mut clk_pin = Output::new(
                unsafe { Peripheral::clone_unchecked(&clk) },
                Level::Low,
                Speed::Low,
            );
            let mut data_pin = Flex::new(unsafe { Peripheral::clone_unchecked(&data) });

            info!("Resetting CS1237");

            // Hold the clock pin high to power off the chip.
            clk_pin.set_high();
            Timer::after(Duration::from_millis(1)).await;

            // Power up the chip by setting the clock pin low.
            clk_pin.set_low();

            // Wait for the chip to be ready.
            let timeout = Duration::from_millis(330);
            with_timeout(timeout, drdy_pin.wait_for_falling_edge())
                .await
                .map_err(|_| Error::Timeout)?;

            info!("Configuring CS1237");

            // Discard the first 29 bits (sample, write status, command follows).
            for _ in 0..29 {
                clk_pin.set_high();
                Timer::after(Duration::from_micros(1)).await;
                clk_pin.set_low();
                Timer::after(Duration::from_micros(1)).await;
            }

            // Set the data pin as an output, now that we're writing to the chip.
            data_pin.set_as_output(Speed::Low);

            // Write the command.
            let command: u8 = 0x65; // Set configuration command.
            for i in (0..7).rev() {
                let bit = (command >> i) & 0x1 != 0;
                data_pin.set_level(if bit { Level::High } else { Level::Low });
                clk_pin.set_high();
                Timer::after(Duration::from_micros(1)).await;
                clk_pin.set_low();
                Timer::after(Duration::from_micros(1)).await;
            }

            // Send gap bit 37.
            clk_pin.set_high();
            Timer::after(Duration::from_micros(1)).await;
            clk_pin.set_low();
            Timer::after(Duration::from_micros(1)).await;
            data_pin.set_level(Level::Low);

            // Write the configuration.
            let config = ((config.sample_rate as u8) << 4)
                | ((config.gain as u8) << 2)
                | (config.channel as u8);
            for i in (0..8).rev() {
                let bit = (config >> i) & 0x1 != 0;
                data_pin.set_level(if bit { Level::High } else { Level::Low });
                clk_pin.set_high();
                Timer::after(Duration::from_micros(1)).await;
                clk_pin.set_low();
                Timer::after(Duration::from_micros(1)).await;
            }

            // Finished writing configuration, set the data pin as an input.
            data_pin.set_as_input(Pull::None);

            // Final clock pulse, bit 46.
            clk_pin.set_high();
            Timer::after(Duration::from_micros(1)).await;
            clk_pin.set_low();
            Timer::after(Duration::from_micros(1)).await;

            info!("Waiting for CS1237 to become ready");

            // Wait for the data pin to go low, will take between 3ms and 300ms
            // Depending on configured sample rate.
            let timeout = Duration::from_millis(330);
            with_timeout(timeout, drdy_pin.wait_for_falling_edge())
                .await
                .map_err(|_| Error::Timeout)?;

            info!("CS1237 configured");
        }

        let spi_dev = Spi::new_rxonly(spi, clk, data, txdma, rxdma, SpiConfig::default());

        Ok(Self { spi_dev, drdy_pin })
    }

    /// Read the next sample from the CS1237 ADC.
    pub async fn read(&mut self) -> Result<i32, Error> {
        // Wait for the interrupt pin to go low.
        let timeout = Duration::from_millis(110);
        with_timeout(timeout, self.drdy_pin.wait_for_falling_edge())
            .await
            .map_err(|_| Error::Timeout)?;

        // Read the data from the cs1237.
        let mut sample = [0u8; 3];
        self.spi_dev
            .transfer_in_place(&mut sample[..])
            .await
            .map_err(|_| Error::SpiError)?;

        Ok(BigEndian::read_i24(&sample))
    }
}
