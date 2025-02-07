//go:build linux

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
