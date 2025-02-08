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

package dhcp

import (
	"context"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/OpenPSG/OpenPSG/recorder/internal/leasedb"
	"github.com/OpenPSG/OpenPSG/recorder/internal/netutil"
	"github.com/insomniacslk/dhcp/dhcpv4"
	"github.com/insomniacslk/dhcp/dhcpv4/server4"
)

// Server is a simple DHCP server that assigns IP addresses to clients.
type Server struct {
	db      *leasedb.DB
	ifname  string
	prefix  netip.Prefix
	gateway netip.Addr
}

func NewServer(db *leasedb.DB, ifname string, prefix netip.Prefix, gateway netip.Addr) *Server {
	return &Server{
		db:      db,
		ifname:  ifname,
		prefix:  prefix,
		gateway: gateway,
	}
}

func (s *Server) ListenAndServe(ctx context.Context) error {
	serverAddr := net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: 67}
	server, err := server4.NewServer(s.ifname, &serverAddr, s.handle)
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()

		if err := server.Close(); err != nil {
			slog.Warn("Failed to close DHCP server", slog.Any("error", err))
		}
	}()

	return server.Serve()
}

func (s *Server) handle(pc net.PacketConn, peer net.Addr, req *dhcpv4.DHCPv4) {
	mac := req.ClientHWAddr

	hostname := req.HostName()
	slog.Debug("Received DHCP message",
		slog.String("mac", mac.String()),
		slog.Any("hostname", hostname),
		slog.Any("messageType", req.MessageType()))

	switch req.MessageType() {
	case dhcpv4.MessageTypeDiscover:
		lease, err := s.db.GetLease(mac)
		if err == nil {
			if lease.ExpiresAt.Before(time.Now()) {
				lease = nil

				if err := s.db.RemoveLease(mac); err != nil {
					slog.Warn("Failed to delete expired lease", slog.Any("error", err))
					return
				}
			}
		}

		if lease == nil {
			// Lease offers are only valid for 5 minutes.
			lease, err = s.db.NewLease(mac, hostname, time.Now().Add(5*time.Minute))
			if err != nil {
				slog.Warn("Failed to assign lease", slog.Any("error", err))
				return
			}
		}

		offer, err := dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			slog.Warn("Failed to create DHCP Offer", slog.Any("error", err))
			return
		}

		offer.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeOffer))
		offer.UpdateOption(dhcpv4.OptServerIdentifier(s.gateway.AsSlice()))
		offer.UpdateOption(dhcpv4.OptRouter(s.gateway.AsSlice()))
		offer.UpdateOption(dhcpv4.OptSubnetMask(netutil.SubnetMask(s.prefix)))
		offer.UpdateOption(dhcpv4.OptDNS(s.gateway.AsSlice()))
		offer.UpdateOption(dhcpv4.OptIPAddressLeaseTime(24 * time.Hour))
		offer.YourIPAddr = net.ParseIP(lease.IPAddress)

		if _, err := pc.WriteTo(offer.ToBytes(), peer); err != nil {
			slog.Warn("Failed to send DHCP Offer", slog.Any("error", err))
		}

	case dhcpv4.MessageTypeRequest:
		lease, err := s.db.GetLease(mac)
		if err != nil {
			slog.Warn("Failed to retrieve lease", slog.Any("error", err))
			return
		}

		if lease.ExpiresAt.Before(time.Now()) {
			slog.Warn("Offer expired", slog.Any("lease", lease))
			if err := s.db.RemoveLease(mac); err != nil {
				slog.Warn("Failed to remove expired lease", slog.Any("error", err))
				return
			}
			return
		}

		// Now that the client has accepted the offer, we can update the lease expiration time.
		lease.ExpiresAt = time.Now().Add(24 * time.Hour)
		if err := s.db.UpdateLease(lease); err != nil {
			slog.Warn("Failed to update lease", slog.Any("error", err))
			return
		}

		ack, err := dhcpv4.NewReplyFromRequest(req)
		if err != nil {
			slog.Warn("Failed to create DHCP ACK", slog.Any("error", err))
			return
		}

		ack.UpdateOption(dhcpv4.OptMessageType(dhcpv4.MessageTypeAck))
		ack.UpdateOption(dhcpv4.OptServerIdentifier(s.gateway.AsSlice()))
		ack.UpdateOption(dhcpv4.OptRouter(s.gateway.AsSlice()))
		ack.UpdateOption(dhcpv4.OptSubnetMask(netutil.SubnetMask(s.prefix)))
		ack.UpdateOption(dhcpv4.OptDNS(s.gateway.AsSlice()))
		ack.UpdateOption(dhcpv4.OptIPAddressLeaseTime(time.Until(lease.ExpiresAt)))
		ack.YourIPAddr = net.ParseIP(lease.IPAddress)

		if _, err := pc.WriteTo(ack.ToBytes(), peer); err != nil {
			slog.Warn("Failed to send DHCP ACK", slog.Any("error", err))
		}

		slog.Debug("Assigned DHCP address to peer",
			slog.String("mac", mac.String()), slog.Any("hostname", hostname), slog.String("address", lease.IPAddress))

	case dhcpv4.MessageTypeNak, dhcpv4.MessageTypeRelease:
		if err := s.db.RemoveLease(mac); err != nil {
			slog.Warn("Failed to remove lease", slog.Any("error", err))
		}

	default:
		slog.Warn("Unhandled DHCP message type", slog.Any("messageType", req.MessageType()))
	}
}
