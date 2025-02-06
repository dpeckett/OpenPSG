use crate::api::SignalValues;
use crate::biquad_filter::BiquadFilter;
use crate::cs1237::Cs1237;
use crate::task::TaskSignal;
use crate::time::clock_gettime;
use defmt::error;
use embassy_futures::select::{select, Either};
use embassy_net::tcp::Error as TcpReadError;
use embassy_sync::blocking_mutex::raw::ThreadModeRawMutex;
use embassy_sync::signal::Signal;
use embedded_jsonrpc::{RpcRequest, RpcServer, JSONRPC_VERSION};
use heapless::Vec;

/// The signal ID for the pressure transducer.
pub(crate) const NCPT_SIGNAL_ID: u32 = 1;

const SAMPLES_PER_SECOND: usize = 40;

// Filter out DC offset, params f0=0.1Hz. This will take ~10s to settle.
const DC_REJECTION_HIGHPASS_FILTER_0_1HZ_NUMERATOR: [f32; 3] = [0.98895425, -1.9779085,   0.98895425];
const DC_REJECTION_HIGHPASS_FILTER_0_1HZ_DENOMINATOR: [f32; 3] = [1.0, -1.97778648, 0.97803051];

// There seems to be some aliasing of mains hum (50Hz) at ~4Hz, this will likely
// need a USA version. Params f0=4.5Hz, Q=0.5
const ANTIALIAS_NOTCH_FILTER_4HZ_NUMERATOR: [f32; 3] = [0.53935085, -0.82025121, 0.53935085];
const ANTIALIAS_NOTCH_FILTER_4HZ_DENOMINATOR: [f32; 3] = [1.0, -0.82025121, 0.07870171];

const I24_MAX: i32 = 8_388_607;

#[embassy_executor::task]
pub async fn sample(
    rpc_server: &'static RpcServer<'static, TcpReadError>,
    signals: &'static Signal<ThreadModeRawMutex, TaskSignal>,
    mut adc: Cs1237<'static>,
) -> ! {
    let mut dc_rejection_filter: BiquadFilter<i32> = BiquadFilter::new(
        DC_REJECTION_HIGHPASS_FILTER_0_1HZ_NUMERATOR,
        DC_REJECTION_HIGHPASS_FILTER_0_1HZ_DENOMINATOR,
    );

    let mut antialias_filter: BiquadFilter<i32> = BiquadFilter::new(
        ANTIALIAS_NOTCH_FILTER_4HZ_NUMERATOR,
        ANTIALIAS_NOTCH_FILTER_4HZ_DENOMINATOR,
    );

    loop {
        // Wait for the start signal
        while signals.wait().await != TaskSignal::Start {}

        // One second of sample data.
        let mut samples = Vec::<i32, SAMPLES_PER_SECOND>::new();
        let mut samples_start = clock_gettime().unwrap();
        let mut scaled_samples = [0; SAMPLES_PER_SECOND];
        loop {
            match select(adc.read(), signals.wait()).await {
                Either::First(Ok(value)) => {
                    samples.push(value).unwrap();

                    if samples.is_full() {
                        // Filter out the DC offset.
                        dc_rejection_filter.apply(samples.as_mut_slice());
                        // Filter out the mains hum alias.
                        antialias_filter.apply(samples.as_mut_slice());

                        // Scale the samples into a 16bit value representing the range -200Pa to 200Pa.
                        // This is the EDF sample value format.
                        for (i, sample) in samples.iter().enumerate() {
                            // 10.4KPa is the full scale range of the pressure transducer 
                            // at this gain setting.
                            let mut pressure_pa: f32 = 10_400.0 * (*sample as f32 / I24_MAX as f32);

                            // Clamp the pressure to the range -200Pa to 200Pa.
                            pressure_pa = pressure_pa.max(-200.0).min(200.0);

                            // Scale to a 16bit signed integer (for EDF).
                            scaled_samples[i] = ((pressure_pa / 200.0) * i16::MAX as f32) as i16;
                        }

                        let notification_payload = &SignalValues {
                            id: NCPT_SIGNAL_ID,
                            timestamp: rfc3339::format_unix(
                                samples_start.seconds,
                                samples_start.micros,
                            ),
                            values: &scaled_samples,
                        };

                        let notification: RpcRequest<&SignalValues<_>> = RpcRequest {
                            jsonrpc: JSONRPC_VERSION,
                            id: None,
                            method: "openpsg.values",
                            params: Some(notification_payload),
                        };

                        let mut notification_json = [0u8; 1460];
                        let notification_len =
                            serde_json_core::to_slice(&notification, &mut notification_json)
                                .unwrap();

                        rpc_server
                            .notify(&notification_json[..notification_len])
                            .await
                            .unwrap();

                        samples_start = clock_gettime().unwrap();
                        samples.clear();
                    }
                }
                Either::First(Err(e)) => {
                    error!("Error reading from ADC: {:?}", e);
                    break;
                }
                Either::Second(sig) => match sig {
                    TaskSignal::Start => continue,
                    TaskSignal::Stop => break,
                },
            }
        }
    }
}
