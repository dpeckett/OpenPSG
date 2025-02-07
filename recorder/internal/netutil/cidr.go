package netutil

import (
	"encoding/binary"
	"net/netip"
)

// BroadcastAddress returns the broadcast address for the given prefix.
func BroadcastAddress(prefix netip.Prefix) netip.Addr {
	addr := prefix.Addr()
	hostBits := addr.BitLen() - prefix.Bits()

	broadcastBytes := make([]byte, addr.BitLen()/8)
	copy(broadcastBytes, addr.AsSlice())

	// Calculate the broadcast address by setting host bits to 1
	if len(broadcastBytes) == 4 {
		ipInt := binary.BigEndian.Uint32(broadcastBytes)
		ipInt |= (1 << hostBits) - 1
		binary.BigEndian.PutUint32(broadcastBytes, ipInt)
	} else {
		// Not implemented for IPv6
		return netip.Addr{}
	}

	broadcastAddr, _ := netip.AddrFromSlice(broadcastBytes)
	return broadcastAddr
}

// SubnetMask returns the subnet mask for the given prefix.
func SubnetMask(prefix netip.Prefix) []byte {
	if !prefix.IsValid() {
		return nil
	}

	ones := prefix.Bits()    // Number of bits in the prefix
	totalBits := 32          // Default for IPv4
	if prefix.Addr().Is6() { // Adjust for IPv6
		totalBits = 128
	}

	// Create a slice with the appropriate number of bytes
	maskBytes := make([]byte, totalBits/8)

	// Fill in the subnet mask
	for i := 0; i < len(maskBytes); i++ {
		if ones >= 8 {
			maskBytes[i] = 0xFF
			ones -= 8
		} else if ones > 0 {
			maskBytes[i] = 0xFF << (8 - ones)
			ones = 0
		}
	}

	return maskBytes
}
