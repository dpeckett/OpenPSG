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

use embassy_stm32::uid::uid;

/// Generate a unique MAC address based on the UID of the device.
pub fn generate_mac_address() -> [u8; 6] {
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
