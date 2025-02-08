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

package leasedb

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"strings"
	"time"

	"github.com/OpenPSG/OpenPSG/recorder/internal/netutil"
	"github.com/miekg/dns"
	bolt "go.etcd.io/bbolt"
)

const (
	configBucketName           = "config"
	leasesBucketName           = "leases"
	leasesByIPBucketName       = "leases_by_ip"
	leasesByHostnameBucketName = "leases_by_hostname"
)

// DB represents a database of DHCP leases.
type DB struct {
	db          *bolt.DB
	gateway     netip.Addr
	prefix      netip.Prefix
	reaperTimer *time.Ticker
}

func Open(dbPath string, prefix netip.Prefix, gateway netip.Addr) (*DB, error) {
	db, err := bolt.Open(dbPath, 0o600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("failed to open lease database: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucketName := range []string{configBucketName, leasesBucketName, leasesByIPBucketName, leasesByHostnameBucketName} {
			_, err := tx.CreateBucketIfNotExists([]byte(bucketName))
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create buckets: %w", err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		configBucket := tx.Bucket([]byte(configBucketName))

		v := configBucket.Get([]byte("prefix"))
		if v == nil {
			return configBucket.Put([]byte("prefix"), []byte(prefix.String()))
		}

		if string(v) != prefix.String() {
			return fmt.Errorf("prefix mismatch: %s != %s", v, prefix.String())
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	ldb := &DB{
		db:          db,
		gateway:     gateway,
		prefix:      prefix,
		reaperTimer: time.NewTicker(5 * time.Minute),
	}

	// Reap any expired leases on startup.
	if err := ldb.ReapExpiredLeases(); err != nil {
		_ = ldb.Close()
		return nil, fmt.Errorf("failed to reap expired leases: %w", err)
	}

	// Start a regular task to reap expired leases.
	go func() {
		for range ldb.reaperTimer.C {
			if err := ldb.ReapExpiredLeases(); err != nil {
				slog.Error("Failed to reap expired leases", slog.Any("error", err))
			}
		}
	}()

	return ldb, nil
}

func (db *DB) Close() error {
	db.reaperTimer.Stop()
	return db.db.Close()
}

type Lease struct {
	MAC       string    `json:"mac"`
	IPAddress string    `json:"ip_address"`
	Hostname  string    `json:"hostname"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewLease creates a new lease for a given MAC address and hostname.
func (db *DB) NewLease(mac net.HardwareAddr, hostname string, expiresAt time.Time) (*Lease, error) {
	var lease *Lease
	err := db.db.Update(func(tx *bolt.Tx) error {
		leasesBucket := tx.Bucket([]byte(leasesBucketName))
		leasesByIPBucket := tx.Bucket([]byte(leasesByIPBucketName))
		leasesByHostnameBucket := tx.Bucket([]byte(leasesByHostnameBucketName))

		// Check if a lease already exists for the MAC address
		if data := leasesBucket.Get(mac); data != nil {
			return fmt.Errorf("lease already exists for MAC: %s", mac)
		}

		// Find the next free IP address
		addr, err := db.nextFreeAddress()
		if err != nil {
			return err
		}

		// Create the lease
		lease = &Lease{
			MAC:       mac.String(),
			IPAddress: addr.String(),
			Hostname:  strings.TrimSuffix(dns.CanonicalName(hostname), "."),
			ExpiresAt: expiresAt,
		}

		// Save the lease
		data, err := json.Marshal(lease)
		if err != nil {
			return err
		}

		if err := leasesBucket.Put(mac, data); err != nil {
			return err
		}

		if err := leasesByIPBucket.Put(addr.AsSlice(), mac); err != nil {
			return err
		}

		if hostname != "" {
			if err := leasesByHostnameBucket.Put([]byte(hostname), mac); err != nil {
				return err
			}
		}

		return nil
	})
	return lease, err
}

// GetLease returns the lease associated with a MAC address.
func (db *DB) GetLease(mac net.HardwareAddr) (*Lease, error) {
	var lease *Lease
	err := db.db.View(func(tx *bolt.Tx) error {
		leasesBucket := tx.Bucket([]byte(leasesBucketName))
		data := leasesBucket.Get(mac)
		if data == nil {
			return fmt.Errorf("lease not found for MAC: %s", mac)
		}

		lease = new(Lease)
		if err := json.Unmarshal(data, lease); err != nil {
			return err
		}

		return nil
	})
	return lease, err
}

// UpdateLease updates the lease associated with a MAC address.
func (db *DB) UpdateLease(lease *Lease) error {
	return db.db.Update(func(tx *bolt.Tx) error {
		leasesBucket := tx.Bucket([]byte(leasesBucketName))
		leasesByIPBucket := tx.Bucket([]byte(leasesByIPBucketName))
		leasesByHostnameBucket := tx.Bucket([]byte(leasesByHostnameBucketName))

		mac, err := net.ParseMAC(lease.MAC)
		if err != nil {
			return err
		}

		data, err := json.Marshal(lease)
		if err != nil {
			return err
		}

		if err := leasesBucket.Put(mac, data); err != nil {
			return err
		}

		if err := leasesByIPBucket.Put(netip.MustParseAddr(lease.IPAddress).AsSlice(), mac); err != nil {
			return err
		}

		if lease.Hostname != "" {
			if err := leasesByHostnameBucket.Put([]byte(lease.Hostname), mac); err != nil {
				return err
			}
		}

		return nil
	})
}

// RemoveLease removes a lease associated with a MAC address.
func (db *DB) RemoveLease(mac net.HardwareAddr) error {
	return db.db.Update(func(tx *bolt.Tx) error {
		leasesBucket := tx.Bucket([]byte(leasesBucketName))
		leasesByIPBucket := tx.Bucket([]byte(leasesByIPBucketName))
		leasesByHostnameBucket := tx.Bucket([]byte(leasesByHostnameBucketName))

		data := leasesBucket.Get(mac)
		if data == nil {
			return fmt.Errorf("lease not found for MAC: %s", mac)
		}

		var lease Lease
		if err := json.Unmarshal(data, &lease); err != nil {
			return err
		}

		if err := leasesBucket.Delete(mac); err != nil {
			return err
		}

		if err := leasesByIPBucket.Delete(netip.MustParseAddr(lease.IPAddress).AsSlice()); err != nil {
			return err
		}

		if lease.Hostname != "" {
			if err := leasesByHostnameBucket.Delete([]byte(lease.Hostname)); err != nil {
				return err
			}
		}

		return nil
	})
}

// ListLeases returns all leases in the database.
func (db *DB) ListLeases() ([]*Lease, error) {
	var leases []*Lease
	err := db.db.View(func(tx *bolt.Tx) error {
		leasesBucket := tx.Bucket([]byte(leasesBucketName))
		c := leasesBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var lease Lease
			if err := json.Unmarshal(v, &lease); err != nil {
				return err
			}
			leases = append(leases, &lease)
		}
		return nil
	})
	return leases, err
}

// ReapExpiredLeases removes all leases that have expired (visible for testing).
func (db *DB) ReapExpiredLeases() error {
	return db.db.Update(func(tx *bolt.Tx) error {
		leasesBucket := tx.Bucket([]byte(leasesBucketName))
		leasesByIPBucket := tx.Bucket([]byte(leasesByIPBucketName))
		leasesByHostnameBucket := tx.Bucket([]byte(leasesByHostnameBucketName))

		c := leasesBucket.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var lease Lease
			if err := json.Unmarshal(v, &lease); err != nil {
				return err
			}

			if lease.ExpiresAt.Before(time.Now()) {
				if err := leasesBucket.Delete(k); err != nil {
					return err
				}

				if err := leasesByIPBucket.Delete(netip.MustParseAddr(lease.IPAddress).AsSlice()); err != nil {
					return err
				}

				if lease.Hostname != "" {
					if err := leasesByHostnameBucket.Delete([]byte(lease.Hostname)); err != nil {
						return err
					}
				}
			}
		}

		return nil
	})
}

func (db *DB) nextFreeAddress() (netip.Addr, error) {
	var addr netip.Addr
	err := db.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(leasesByIPBucketName))

		// Start from the first valid address in the prefix
		addr = db.prefix.Addr()
		if addr.Is4() && addr.As4()[3] == 0 {
			addr = addr.Next()
		}

		broadcastAddr := netutil.BroadcastAddress(db.prefix)

		for ; db.prefix.Contains(addr); addr = addr.Next() {
			if addr == db.gateway || addr == broadcastAddr {
				continue
			}

			if b.Get([]byte(addr.String())) == nil {
				return nil
			}
		}

		return fmt.Errorf("no free IP addresses")
	})
	return addr, err
}
