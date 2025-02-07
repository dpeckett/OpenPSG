package openpsg

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"time"

	"github.com/sourcegraph/jsonrpc2"
)

const timeout = 5 * time.Second

type Client struct {
	rpcConn      *jsonrpc2.Conn
	signalValues chan SignalValues
}

// Connect to the device at the specified address and port.
func Connect(ctx context.Context, deviceAddrPort netip.AddrPort) (*Client, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", deviceAddrPort.String())
	if err != nil {
		return nil, fmt.Errorf("failed to connect to device: %w", err)
	}

	c := Client{
		signalValues: make(chan SignalValues),
	}
	c.rpcConn = jsonrpc2.NewConn(ctx, jsonrpc2.NewBufferedStream(conn, jsonrpc2.VSCodeObjectCodec{}), &c)
	return &c, nil
}

func (c *Client) Close() error {
	err := c.rpcConn.Close()
	close(c.signalValues)
	return err
}

// Retrieve the list of signals available on the device.
func (c *Client) Signals(ctx context.Context) ([]Signal, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var signals []Signal
	if err := c.rpcConn.Call(ctx, "openpsg.signals", nil, &signals); err != nil {
		return nil, fmt.Errorf("failed to get signals: %w", err)
	}
	return signals, nil
}

// Start collecting data for the specified signals.
func (c *Client) Start(ctx context.Context, signalIDs []uint32) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.rpcConn.Notify(ctx, "openpsg.start", signalIDs)
}

// Stop collecting data for the specified signals.
func (c *Client) Stop(ctx context.Context, signalIDs []uint32) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return c.rpcConn.Notify(ctx, "openpsg.stop", signalIDs)
}

// SignalValues returns a channel that will receive the values of the signals.
func (c *Client) SignalValues() <-chan SignalValues {
	return c.signalValues
}

// Handle a notification from the server.
func (c *Client) Handle(ctx context.Context, conn *jsonrpc2.Conn, r *jsonrpc2.Request) {
	switch r.Method {
	case "openpsg.values":
		var values SignalValues
		if err := json.Unmarshal(*r.Params, &values); err != nil {
			slog.Error("Failed to unmarshal values", slog.Any("error", err))
			return
		}

		c.signalValues <- values
	default:
		slog.Warn("Unknown notification received", slog.String("method", r.Method))
	}
}
