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

package openpsg

import (
	"context"
	"fmt"
	"log/slog"
	"net/netip"
	"os"
	"strings"
	"time"

	"github.com/OpenPSG/OpenPSG/recorder/internal/leasedb"
	"github.com/OpenPSG/OpenPSG/recorder/internal/termutil"
	"github.com/olekukonko/tablewriter"
	"golang.org/x/term"
)

// Discover scans the network for sensor devices and returns a list of their IP addresses.
func Discover(ctx context.Context, db *leasedb.DB) ([]netip.Addr, error) {
	discoverComplete := make(chan struct{})

	// Start a goroutine to listen for key presses.
	go func() {
		defer close(discoverComplete)

		_, err := term.ReadPassword(int(os.Stdin.Fd()))
		if err != nil {
			slog.Warn("Failed to read from stdin", slog.Any("error", err))
		}
	}()

	// Create a new ASCII table for the current leases
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"MAC Address", "IP Address", "Hostname", "Signals", "Status"})
	table.SetBorder(false)

	firstScan := true
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var deviceAddrs []netip.Addr
	for {
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-discoverComplete:
			return deviceAddrs, nil
		case <-ticker.C:
		}

		leases, err := db.ListLeases()
		if err != nil {
			return nil, fmt.Errorf("failed to list leases: %w", err)
		}

		if !firstScan {
			table.ClearRows()
		}

		deviceAddrs = deviceAddrs[:0]

		for _, lease := range leases {
			deviceAddr := netip.MustParseAddr(lease.IPAddress)

			var signalNames []string
			status := "Offline"

			client, err := Connect(ctx, netip.AddrPortFrom(deviceAddr, 80))
			if err == nil {
				signals, err := client.Signals(ctx)
				_ = client.Close()
				if err == nil {
					for _, signal := range signals {
						signalNames = append(signalNames, signal.Name)
					}
					status = "Online"
				}
			}

			table.Append([]string{
				lease.MAC,
				lease.IPAddress,
				lease.Hostname,
				strings.Join(signalNames, ", "),
				status,
			})

			if status == "Online" {
				deviceAddrs = append(deviceAddrs, deviceAddr)
			}
		}

		if !firstScan {
			termutil.ClearLines(table.NumLines() + 3)
		}

		table.Render()
		fmt.Println("Press Enter to stop scanning for devices ...")
		firstScan = false
	}
}
