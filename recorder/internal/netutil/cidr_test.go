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

package netutil_test

import (
	"net/netip"
	"testing"

	"github.com/OpenPSG/OpenPSG/recorder/internal/netutil"
	"github.com/stretchr/testify/assert"
)

func TestBroadcastAddress(t *testing.T) {
	t.Run("IPv4 /24", func(t *testing.T) {
		prefix := netip.MustParsePrefix("192.168.1.0/24")
		addr := netutil.BroadcastAddress(prefix)
		expect := netip.MustParseAddr("192.168.1.255")

		assert.Equal(t, expect, addr)
	})

	t.Run("IPv4 /16", func(t *testing.T) {
		prefix := netip.MustParsePrefix("10.0.0.0/16")
		addr := netutil.BroadcastAddress(prefix)
		expect := netip.MustParseAddr("10.0.255.255")

		assert.Equal(t, expect, addr)
	})
}

func TestSubnetMask(t *testing.T) {
	t.Run("IPv4 /24", func(t *testing.T) {
		prefix := netip.MustParsePrefix("192.168.1.0/24")
		mask, _ := netip.AddrFromSlice(netutil.SubnetMask(prefix))
		expect := "255.255.255.0"

		assert.Equal(t, expect, mask.String())
	})

	t.Run("IPv4 /16", func(t *testing.T) {
		prefix := netip.MustParsePrefix("10.0.0.0/16")
		mask, _ := netip.AddrFromSlice(netutil.SubnetMask(prefix))
		expect := "255.255.0.0"

		assert.Equal(t, expect, mask.String())
	})

	t.Run("IPv6 /64", func(t *testing.T) {
		prefix := netip.MustParsePrefix("2001:db8::/64")
		mask, _ := netip.AddrFromSlice(netutil.SubnetMask(prefix))
		expect := "ffff:ffff:ffff:ffff::"

		assert.Equal(t, expect, mask.String())
	})

	t.Run("IPv6 /128 (Single Address)", func(t *testing.T) {
		prefix := netip.MustParsePrefix("2001:db8::1/128")
		mask, _ := netip.AddrFromSlice(netutil.SubnetMask(prefix))
		expect := "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff"

		assert.Equal(t, expect, mask.String())
	})
}
