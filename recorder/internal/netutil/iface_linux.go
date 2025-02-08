//go:build linux

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

package netutil

import (
	"fmt"
	"net/netip"

	"github.com/vishvananda/netlink"
)

// ConfigureNetworkInterface brings up the network interface with the given name
// and assigns it the given IP address and network prefix.
func ConfigureNetworkInterface(ifname string, gateway netip.Addr, prefix netip.Prefix) error {
	link, err := netlink.LinkByName(ifname)
	if err != nil {
		return fmt.Errorf("failed to find interace with name %s: %w", ifname, err)
	}

	addr, err := netlink.ParseAddr(netip.PrefixFrom(gateway, prefix.Bits()).String())
	if err != nil {
		return fmt.Errorf("failed to parse address: %w", err)
	}

	if err := netlink.AddrAdd(link, addr); err != nil && err.Error() != "file exists" {
		return fmt.Errorf("failed to add address to interface: %w", err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring interface up: %w", err)
	}

	return nil
}
