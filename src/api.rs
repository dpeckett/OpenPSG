use crate::ncpt;
use crate::task::TaskSignal;
use core::fmt::Debug;
use defmt::warn;
use embassy_sync::blocking_mutex::raw::ThreadModeRawMutex;
use embassy_sync::signal::Signal as EmbassySignal;
use embedded_jsonrpc::{
    stackfuture::StackFuture, RpcError, RpcErrorCode, RpcResponse, DEFAULT_HANDLER_STACK_SIZE,
    JSONRPC_VERSION,
};
use heapless::String;
use heapless::Vec;
use rfc3339::Timestamp;
use serde::de::Deserializer;
use serde::{Deserialize, Serialize};

/// The transducer type used to measure a signal.
#[derive(Debug, Deserialize, Serialize)]
enum TransducerType {
    #[serde(rename = "MEMSPressureTransducer")]
    MEMSPressureTransducer,
}

/// The unit of a signal.
#[derive(Clone, Copy, Debug, Deserialize, Serialize)]
enum Unit {
    #[serde(rename = "uV")]
    Microvolts,
    #[serde(rename = "mV")]
    Millivolts,
    #[serde(rename = "V")]
    Volts,
    #[serde(rename = "Hz")]
    Hertz,
    #[serde(rename = "kHz")]
    Kilohertz,
    #[serde(rename = "Pa")]
    Pascals,
}

impl Unit {
    fn to_string(self) -> &'static str {
        match self {
            Unit::Microvolts => "uV",
            Unit::Millivolts => "mV",
            Unit::Volts => "V",
            Unit::Hertz => "Hz",
            Unit::Kilohertz => "kHz",
            Unit::Pascals => "Pa",
        }
    }
}

impl From<&str> for Unit {
    fn from(s: &str) -> Self {
        match s {
            "uV" => Unit::Microvolts,
            "mV" => Unit::Millivolts,
            "V" => Unit::Volts,
            "Hz" => Unit::Hertz,
            "kHz" => Unit::Kilohertz,
            "Pa" => Unit::Pascals,
            _ => panic!("unknown unit"),
        }
    }
}

/// The kind of filter applied to a signal.
#[derive(Clone, Copy, Debug)]
enum FilterKind {
    HighPass,
    LowPass,
    Notch,
}

/// A filtering operation applied to a signal.
#[derive(Clone, Copy, Debug)]
struct Filter {
    kind: FilterKind,
    unit: Unit,
    frequency: f32,
}

const MAX_FILTERS: usize = 8;

#[derive(Debug)]
struct FilterList {
    filters: Vec<Filter, MAX_FILTERS>,
}

impl<'de> serde::Deserialize<'de> for FilterList {
    fn deserialize<D: Deserializer<'de>>(d: D) -> Result<Self, D::Error> {
        let filters_str = String::<64>::deserialize(d)?;

        let mut filters = Vec::<Filter, MAX_FILTERS>::new();
        for filter_str in filters_str.split(' ') {
            let mut parts = filter_str.split(':');
            let kind = match parts
                .next()
                .ok_or_else(|| serde::de::Error::custom("missing kind"))?
            {
                "HP" => FilterKind::HighPass,
                "LP" => FilterKind::LowPass,
                "N" => FilterKind::Notch,
                _ => return Err(serde::de::Error::custom("unknown filter kind")),
            };

            let mut frequency_and_unit = parts
                .next()
                .ok_or_else(|| serde::de::Error::custom("missing frequency and unit"))?
                .split(|c: char| !c.is_ascii_digit() && c != '.');

            let frequency_str = frequency_and_unit
                .next()
                .ok_or_else(|| serde::de::Error::custom("missing frequency"))?;

            let unit_str = frequency_and_unit
                .next()
                .ok_or_else(|| serde::de::Error::custom("missing unit"))?;

            let frequency = frequency_str
                .parse::<f32>()
                .map_err(|_| serde::de::Error::custom("invalid frequency"))?;

            let unit: Unit = unit_str.into();

            filters
                .push(Filter {
                    kind,
                    unit,
                    frequency,
                })
                .map_err(|_| serde::de::Error::custom("too many filters"))?;
        }

        Ok(FilterList { filters })
    }
}

impl serde::Serialize for FilterList {
    fn serialize<S: serde::Serializer>(&self, s: S) -> Result<S::Ok, S::Error> {
        let mut filters_str: String<64> = String::new();
        for filter in &self.filters {
            let kind_str = match filter.kind {
                FilterKind::HighPass => "HP",
                FilterKind::LowPass => "LP",
                FilterKind::Notch => "N",
            };

            let mut filter_str: String<16> = String::new();
            core::fmt::write(
                &mut filter_str,
                format_args!(
                    "{}:{:.2}{}",
                    kind_str,
                    filter.frequency,
                    filter.unit.to_string()
                ),
            )
            .map_err(|_| serde::ser::Error::custom("formatting error"))?;

            if !filters_str.is_empty() {
                filters_str
                    .push(' ')
                    .map_err(|_| serde::ser::Error::custom("too many filters"))?;
            }

            filters_str
                .push_str(&filter_str)
                .map_err(|_| serde::ser::Error::custom("too many filters"))?;
        }

        filters_str.serialize(s)
    }
}

#[derive(Debug, Serialize)]
struct Signal<'a> {
    /// The unique identifier of the signal.
    id: u32,
    /// The human-readable name of the signal.
    name: &'a str,
    /// The type of transducer used to measure the signal.
    #[serde(rename(serialize = "transducerType"))]
    transducer_type: TransducerType,
    /// The unit of the signal (eg. microvolts).
    unit: Unit,
    /// The minimum value of the signal (in the unit of the signal).
    min: f32,
    /// The maximum value of the signal (in the unit of the signal).
    max: f32,
    /// The list of filters applied to the signal.
    prefiltering: FilterList,
    /// The sample rate of the signal (in Hertz).
    #[serde(rename(serialize = "sampleRate"))]
    sample_rate: u32,
}

/// The values of a signal at a given timestamp.
#[derive(Debug, Serialize)]
pub struct SignalValues<'a, T: Serialize> {
    /// The unique identifier of the signal these values belong to.
    pub id: u32,
    /// The start timestamp of the values.
    pub timestamp: Timestamp,
    /// The list of values.
    pub values: &'a [T],
}

pub struct RpcHandler {
    ncpt_sampling_task_signals:
        &'static EmbassySignal<ThreadModeRawMutex, TaskSignal>,
}

impl RpcHandler {
    pub fn new(
        ncpt_sampling_task_signals: &'static EmbassySignal<
            ThreadModeRawMutex,
            TaskSignal,
        >,
    ) -> Self {
        Self {
            ncpt_sampling_task_signals,
        }
    }

    async fn signals<'a>(
        &self,
        id: Option<u64>,
        response_json: &'a mut [u8],
    ) -> Result<usize, RpcError> {
        let signals: [Signal; 1] = [Signal {
            id: ncpt::NCPT_SIGNAL_ID,
            name: "Nasal Pressure",
            transducer_type: TransducerType::MEMSPressureTransducer,
            unit: Unit::Pascals,
            min: -200.0,
            max: 200.0,
            prefiltering: FilterList {
                filters: Vec::from_slice(&[
                    Filter {
                        kind: FilterKind::HighPass,
                        unit: Unit::Hertz,
                        frequency: 0.1,
                    },
                    Filter {
                        kind: FilterKind::Notch,
                        unit: Unit::Hertz,
                        frequency: 4.0,
                    },
                ])
                .unwrap(),
            },
            sample_rate: 40,
        }];

        let response: RpcResponse<&[Signal]> = RpcResponse {
            jsonrpc: JSONRPC_VERSION,
            error: None,
            result: Some(&signals),
            id,
        };

        Ok(serde_json_core::to_slice(&response, response_json).unwrap())
    }

    async fn start<'a>(
        &self,
        id: Option<u64>,
        request_json: &'a [u8],
        response_json: &'a mut [u8],
    ) -> Result<usize, RpcError> {
        #[derive(Debug, Deserialize, defmt::Format)]
        struct SignalIdsRequest {
            #[serde(rename = "params")]
            signal_ids: Vec<u32, 1>,
        }

        let request: SignalIdsRequest = match serde_json_core::from_slice(request_json) {
            Ok((request, _remainder)) => request,
            Err(_) => {
                warn!("Unable to parse request");
                return Err(RpcErrorCode::InvalidParams.into());
            }
        };

        if request.signal_ids.len() != 1 || request.signal_ids[0] != 1 {
            warn!("Invalid request: {}", request);
            return Err(RpcErrorCode::InvalidParams.into());
        }

        // Start sampling.
        self.ncpt_sampling_task_signals
            .signal(TaskSignal::Start);

        let response: RpcResponse<'static, ()> = RpcResponse {
            jsonrpc: JSONRPC_VERSION,
            error: None,
            result: None,
            id,
        };

        Ok(serde_json_core::to_slice(&response, response_json).unwrap())
    }

    async fn stop<'a>(
        &self,
        id: Option<u64>,
        request_json: &'a [u8],
        response_json: &'a mut [u8],
    ) -> Result<usize, RpcError> {
        #[derive(Debug, Deserialize, defmt::Format)]
        struct SignalIdsRequest {
            #[serde(rename = "params")]
            signal_ids: Vec<u32, 1>,
        }

        let request: SignalIdsRequest = match serde_json_core::from_slice(request_json) {
            Ok((request, _remainder)) => request,
            Err(_) => {
                warn!("Unable to parse request");
                return Err(RpcErrorCode::InvalidParams.into());
            }
        };

        if request.signal_ids.len() != 1 || request.signal_ids[0] != 1 {
            warn!("Invalid request: {}", request);
            return Err(RpcErrorCode::InvalidParams.into());
        }

        // Stop sampling.
        self.ncpt_sampling_task_signals
            .signal(TaskSignal::Stop);

        let response: RpcResponse<'static, ()> = RpcResponse {
            jsonrpc: JSONRPC_VERSION,
            error: None,
            result: None,
            id,
        };

        Ok(serde_json_core::to_slice(&response, response_json).unwrap())
    }
}

impl embedded_jsonrpc::RpcHandler for RpcHandler {
    fn handle<'a>(
        &'a self,
        id: Option<u64>,
        method: &'a str,
        request_json: &'a [u8],
        response_json: &'a mut [u8],
    ) -> StackFuture<'a, Result<usize, RpcError>, DEFAULT_HANDLER_STACK_SIZE> {
        StackFuture::from(async move {
            match method {
                "openpsg.signals" => self.signals(id, response_json).await,
                "openpsg.start" => self.start(id, request_json, response_json).await,
                "openpsg.stop" => self.stop(id, request_json, response_json).await,
                _ => Err(RpcErrorCode::MethodNotFound.into()),
            }
        })
    }
}
