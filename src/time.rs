/* SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Copyright (C) 2025 The OpenPSG Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

use chrono::{Datelike, NaiveDate, NaiveDateTime, NaiveTime, Timelike, Weekday};
use core::cell::RefCell;
use core::result::Result;
use embassy_stm32::pac;
use embassy_stm32::rtc::{DateTime, DayOfWeek, Rtc, RtcConfig, RtcError};
use embassy_sync::blocking_mutex::{raw::ThreadModeRawMutex, Mutex};

static RTC: Mutex<ThreadModeRawMutex, RefCell<Option<Rtc>>> = Mutex::new(RefCell::new(None));
static MICROS_OFFSET: Mutex<ThreadModeRawMutex, RefCell<u32>> = Mutex::new(RefCell::new(0));

/// A timespec structure representing a time in seconds and microseconds.
pub struct Timespec {
    pub seconds: u64,
    pub micros: u32,
}

/// Initialize the real-time clock peripheral (RTC).
pub fn init_time(rtc: impl embassy_stm32::Peripheral<P = embassy_stm32::peripherals::RTC>) {
    RTC.lock(|rc| {
        *rc.borrow_mut() = Some(Rtc::new(rtc, RtcConfig::default()));
    });
}

/// Set the current time of the RTC from a unix epoch timestamp.
pub fn clock_settime(tp: &Timespec) -> Result<(), RtcError> {
    let datetime = chrono::DateTime::from_timestamp(tp.seconds as i64, tp.micros * 1000)
        .unwrap()
        .naive_utc();
    let date = datetime.date();
    let time = datetime.time();

    let day_of_week = match date.weekday() {
        Weekday::Mon => DayOfWeek::Monday,
        Weekday::Tue => DayOfWeek::Tuesday,
        Weekday::Wed => DayOfWeek::Wednesday,
        Weekday::Thu => DayOfWeek::Thursday,
        Weekday::Fri => DayOfWeek::Friday,
        Weekday::Sat => DayOfWeek::Saturday,
        Weekday::Sun => DayOfWeek::Sunday,
    };

    let new_date_time = DateTime::from(
        date.year() as u16,
        date.month() as u8,
        date.day() as u8,
        day_of_week,
        time.hour() as u8,
        time.minute() as u8,
        time.second() as u8,
    )
    .unwrap();

    RTC.lock(|rc| -> Result<(), RtcError> {
        if let Some(rtc) = &mut *rc.borrow_mut() {
            rtc.set_datetime(new_date_time)?;

            // set_datetime() clears the subsecond time register, so store
            // a copy of the microseconds offset.
            MICROS_OFFSET.lock(|micros_offset| {
                *micros_offset.borrow_mut() = tp.micros;
            });
        } else {
            return Err(RtcError::NotRunning);
        }

        Ok(())
    })
}

/// Get the current time of the RTC as a unix epoch timestamp.
pub fn clock_gettime() -> Result<Timespec, RtcError> {
    RTC.lock(|rc| -> Result<Timespec, RtcError> {
        if let Some(rtc) = &*rc.borrow() {
            let now = rtc.now()?;
            let now_micros = get_rtc_micros() as i64;

            // Restore the microseconds offset
            let micros_offset = MICROS_OFFSET.lock(|micros_offset| *micros_offset.borrow()) as i64;

            let naive_date_time = NaiveDateTime::new(
                NaiveDate::from_ymd_opt(now.year() as i32, now.month() as u32, now.day() as u32)
                    .unwrap(),
                NaiveTime::from_hms_opt(
                    now.hour() as u32,
                    now.minute() as u32,
                    now.second() as u32,
                )
                .unwrap(),
            )
            .checked_add_signed(chrono::Duration::microseconds(now_micros + micros_offset))
            .unwrap();

            let seconds = naive_date_time.and_utc().timestamp();
            let micros = naive_date_time.and_utc().timestamp_subsec_micros() % 1_000_000;

            Ok(Timespec {
                seconds: seconds as u64,
                micros,
            })
        } else {
            Err(RtcError::NotRunning)
        }
    })
}

// Calculate the subsecond time in microseconds
// This is not exposed by the embassy-stm32 crate at the moment.
// When https://github.com/embassy-rs/embassy/issues/2377 is
// resolved, this can be removed.
fn get_rtc_micros() -> u32 {
    let raw_subsecond = pac::RTC.ssr().read().ss() as u32;
    let prediv_s = pac::RTC.prer().read().prediv_s() as u32;
    ((prediv_s - raw_subsecond) * 1_000_000) / (prediv_s + 1)
}
