use core::cell::RefCell;
use core::fmt::Debug;
use embassy_sync::blocking_mutex::raw::ThreadModeRawMutex;
use embassy_sync::blocking_mutex::Mutex;
use embassy_time::{Duration, Instant};
use sntpc::NtpResult;
use sntpc::NtpTimestampGenerator;

static UNIX_EPOCH_OFFSET: Mutex<ThreadModeRawMutex, RefCell<Duration>> =
    Mutex::new(RefCell::new(Duration::from_micros(0)));

fn initialize_timestamp_gen() {
    UNIX_EPOCH_OFFSET.lock(|epoch_offset| {
        *epoch_offset.borrow_mut() =
            Duration::from_secs(env!("UNIX_EPOCH_OFFSET").parse::<u64>().unwrap_or_default());
    });
}

pub fn set_time(time: NtpResult) {
    let seconds = time.sec();
    let micros = u64::from(time.sec_fraction()) * 1_000_000 / u64::from(u32::MAX);

    UNIX_EPOCH_OFFSET.lock(|epoch_offset| {
        *epoch_offset.borrow_mut() = Duration::from_micros(
            (seconds as u64 * 1_000_000 + micros) - Instant::now().as_micros(),
        );
    });
}

// Returns the current time.
pub fn get_time() -> Instant {
    let mut epoch_offset = UNIX_EPOCH_OFFSET.lock(|d: &RefCell<Duration>| *d.borrow());
    if epoch_offset.as_micros() == 0 {
        initialize_timestamp_gen();
        epoch_offset = UNIX_EPOCH_OFFSET.lock(|d| *d.borrow());
    }
    Instant::now().checked_add(epoch_offset).unwrap()
}

#[derive(Debug, Clone, Copy)]
pub struct TimestampGen {
    now: Instant,
}

impl Default for TimestampGen {
    fn default() -> Self {
        Self {
            now: Instant::now(),
        }
    }
}

impl NtpTimestampGenerator for TimestampGen {
    /// Initialize timestamp generator state with `now` system time since UNIX EPOCH.
    /// Expected to be called every time before `timestamp_sec` and
    /// `timestamp_subsec_micros` usage.
    fn init(&mut self) {
        let epoch_offset = UNIX_EPOCH_OFFSET.lock(|d| *d.borrow());
        if epoch_offset.as_micros() == 0 {
            initialize_timestamp_gen();
        }
        self.now = Instant::now();
    }

    /// Returns timestamp in seconds since UNIX EPOCH.
    fn timestamp_sec(&self) -> u64 {
        let epoch_offset = UNIX_EPOCH_OFFSET.lock(|d| *d.borrow());
        let seconds: u64 = self.now.checked_add(epoch_offset).unwrap().as_secs();
        seconds
    }

    /// Returns the fractional part of the timestamp in whole micro seconds.
    fn timestamp_subsec_micros(&self) -> u32 {
        let epoch_offset = UNIX_EPOCH_OFFSET.lock(|d| *d.borrow());
        let micros = (self.now.checked_add(epoch_offset).unwrap().as_micros() % 1_000_000) as u32;
        micros
    }
}
