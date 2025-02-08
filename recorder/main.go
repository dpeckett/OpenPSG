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

package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"syscall"

	"log/slog"

	"github.com/OpenPSG/OpenPSG/recorder/internal/dhcp"
	"github.com/OpenPSG/OpenPSG/recorder/internal/leasedb"
	"github.com/OpenPSG/OpenPSG/recorder/internal/netutil"
	"github.com/OpenPSG/OpenPSG/recorder/openpsg"
	"github.com/OpenPSG/sntp"
	"github.com/adrg/xdg"
	"github.com/urfave/cli/v2"
	"golang.org/x/sync/errgroup"
)

func main() {
	// Store DHCP leases in the XDG data directory (if available).
	dbPath, err := xdg.DataFile("openpsg-recorder/dhcp_leases.db")
	if err != nil {
		slog.Warn("Failed to get default database path", slog.Any("error", err))
		dbPath = "dhcp_leases.db"
	}

	sharedFlags := []cli.Flag{
		&cli.StringFlag{
			Name:  "log-level",
			Value: "info",
			Usage: "Log level (debug, info, warn, error)",
		},
		&cli.StringFlag{
			Name:  "db-path",
			Value: dbPath,
			Usage: "Path to the DHCP lease database",
		},
	}

	app := &cli.App{
		Name:  "openpsg-recorder",
		Usage: "Records PSG data from one or more Ethernet sensors",
		Flags: append([]cli.Flag{
			&cli.StringFlag{
				Name:     "interface",
				Aliases:  []string{"i"},
				Usage:    "Network interface name",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "prefix",
				Value: "10.24.0.0/24",
				Usage: "CIDR prefix for the network",
			},
			&cli.StringFlag{
				Name:  "gateway",
				Value: "10.24.0.1",
				Usage: "Gateway IP address",
			},
			&cli.StringFlag{
				Name:    "output",
				Aliases: []string{"o"},
				Value:   "openpsg.edf",
				Usage:   "Output file for the recording",
			},
			&cli.StringFlag{
				Name:    "patient-id",
				Aliases: []string{"p"},
				Value:   "X",
				Usage:   "Patient ID for the recording",
			},
			&cli.StringFlag{
				Name:    "recording-id",
				Aliases: []string{"r"},
				Value:   "1",
				Usage:   "Recording ID for the recording",
			},
		}, sharedFlags...),
		Action: func(c *cli.Context) error {
			// Configure the logger.
			var logLevel slog.Level
			if err := logLevel.UnmarshalText([]byte(c.String("log-level"))); err != nil {
				return fmt.Errorf("failed to parse log level: %w", err)
			}
			slog.SetLogLoggerLevel(logLevel)

			ifname := c.String("interface")

			prefix, err := netip.ParsePrefix(c.String("prefix"))
			if err != nil {
				return fmt.Errorf("failed to parse network prefix: %w", err)
			}

			gateway, err := netip.ParseAddr(c.String("gateway"))
			if err != nil {
				return fmt.Errorf("failed to parse network gateway address: %w", err)
			}

			// Configure the network interface.
			if err := netutil.ConfigureNetworkInterface(ifname, gateway, prefix); err != nil {
				return fmt.Errorf("failed to setup interface: %w", err)
			}

			// Open the DHCP lease database.
			db, err := leasedb.Open(c.String("db-path"), prefix, gateway)
			if err != nil {
				return fmt.Errorf("failed to open dhcp lease database: %w", err)
			}
			defer db.Close()

			g, ctx := errgroup.WithContext(appContext(c.Context))

			// Set up the DHCP server.
			dhcpServer := dhcp.NewServer(db, ifname, prefix, gateway)
			g.Go(func() error {
				slog.Debug("Starting DHCP server",
					slog.String("interface", ifname),
					slog.Any("prefix", prefix),
					slog.Any("gateway", gateway))

				err := dhcpServer.ListenAndServe(ctx)
				if err != nil && !errors.Is(err, net.ErrClosed) {
					return fmt.Errorf("failed to run DHCP server: %w", err)
				}

				return nil
			})

			// Set up the NTP server
			ntpServer := sntp.NewServer()
			g.Go(func() error {
				slog.Debug("Starting NTP server")

				err := ntpServer.ListenAndServe(ctx, net.JoinHostPort(gateway.String(), "123"))
				if err != nil && !errors.Is(err, net.ErrClosed) {
					return fmt.Errorf("failed to run NTP server: %w", err)
				}

				return nil
			})

			g.Go(func() error {
				slog.Info("Discovering devices ...")

				deviceAddrs, err := openpsg.Discover(ctx, db)
				if err != nil {
					return fmt.Errorf("failed to discover devices: %w", err)
				}

				slog.Info("Recording from devices", slog.Any("deviceAddrs", deviceAddrs))

				f, err := os.Create(c.String("output"))
				if err != nil {
					return fmt.Errorf("failed to create file: %w", err)
				}
				defer f.Close()

				if err := openpsg.Record(ctx, f, c.String("patient-id"), c.String("recording-id"), deviceAddrs); err != nil {
					return fmt.Errorf("failed to record from devices: %w", err)
				}

				return nil
			})

			return g.Wait()
		},
	}

	if err := app.Run(os.Args); err != nil {
		slog.Error("Error running app", slog.Any("error", err))
		os.Exit(1)
	}
}

// signal aware context cancellation.
func appContext(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		s := <-sigs
		slog.Info("Received signal, shutting down ...", slog.String("signal", s.String()))
		cancel()
	}()

	return ctx
}
