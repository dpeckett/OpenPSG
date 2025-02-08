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

package leasedb_test

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"net/netip"

	"github.com/OpenPSG/OpenPSG/recorder/internal/leasedb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLeaseDB(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	gateway := netip.MustParseAddr("192.168.1.1")

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "leases.db")

	db, err := leasedb.Open(dbPath, prefix, gateway)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	t.Run("TestNewLease", func(t *testing.T) {
		mac := net.HardwareAddr{0x00, 0x1B, 0x2C, 0x3D, 0x4E, 0x5F}
		hostname := "test-host-1"

		lease, err := db.NewLease(mac, hostname, time.Now().Add(24*time.Hour))
		require.NoError(t, err)

		assert.Equal(t, "00:1b:2c:3d:4e:5f", lease.MAC)
		assert.NotEmpty(t, lease.IPAddress)
		assert.Equal(t, hostname, lease.Hostname)
		assert.WithinDuration(t, time.Now().Add(24*time.Hour), lease.ExpiresAt, time.Minute)
	})

	t.Run("TestGetLease", func(t *testing.T) {
		mac := net.HardwareAddr{0x00, 0x1C, 0x2D, 0x3E, 0x4F, 0x60}
		hostname := "test-host-2"

		_, err := db.NewLease(mac, hostname, time.Now().Add(24*time.Hour))
		require.NoError(t, err)

		lease, err := db.GetLease(mac)
		require.NoError(t, err)

		assert.Equal(t, "00:1c:2d:3e:4f:60", lease.MAC)
		assert.Equal(t, hostname, lease.Hostname)
	})

	t.Run("TestUpdateLease", func(t *testing.T) {
		mac := net.HardwareAddr{0x00, 0x1E, 0x2F, 0x3A, 0x4B, 0x5C}
		hostname := "test-host-4"

		lease, err := db.NewLease(mac, hostname, time.Now().Add(24*time.Hour))
		require.NoError(t, err)

		assert.Equal(t, "00:1e:2f:3a:4b:5c", lease.MAC)
		assert.Equal(t, hostname, lease.Hostname)

		updatedHostname := "updated-host"
		updatedExpiration := time.Now().Add(48 * time.Hour)
		lease.Hostname = updatedHostname
		lease.ExpiresAt = updatedExpiration

		err = db.UpdateLease(lease)
		require.NoError(t, err)

		updatedLease, err := db.GetLease(mac)
		require.NoError(t, err)

		assert.Equal(t, updatedHostname, updatedLease.Hostname)
		assert.WithinDuration(t, updatedExpiration, updatedLease.ExpiresAt, time.Minute)
	})

	t.Run("TestRemoveLease", func(t *testing.T) {
		mac := net.HardwareAddr{0x00, 0x1D, 0x2E, 0x3F, 0x50, 0x61}
		hostname := "test-host-3"

		_, err := db.NewLease(mac, hostname, time.Now().Add(24*time.Hour))
		require.NoError(t, err)

		err = db.RemoveLease(mac)
		require.NoError(t, err)

		_, err = db.GetLease(mac)
		assert.Error(t, err, "expected error when retrieving a removed lease")
	})
}

func TestLeaseDB_ReapExpiredLeases(t *testing.T) {
	prefix := netip.MustParsePrefix("192.168.1.0/24")
	gateway := netip.MustParseAddr("192.168.1.1")

	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "leases.db")

	db, err := leasedb.Open(dbPath, prefix, gateway)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	mac := net.HardwareAddr{0x00, 0x1A, 0x2B, 0x3C, 0x4D, 0x5E}
	hostname := "test-host"

	_, err = db.NewLease(mac, hostname, time.Now().Add(-time.Minute))
	require.NoError(t, err)

	_, err = db.GetLease(mac)
	require.NoError(t, err)

	err = db.ReapExpiredLeases()
	require.NoError(t, err)

	_, err = db.GetLease(mac)
	assert.Error(t, err, "expected error when retrieving an expired lease")
}
