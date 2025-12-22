/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package netutil

import (
	"context"
	"net"
	"sync/atomic"
	"time"
)

// NewCustomDNSResolver creates a new CustomDNSResolver.
//
// Example of usage with pq:
//
//	func main() {
//		addrs := []string{
//			"127.0.0.1:53",
//			"192.168.1.1:53",
//		}
//		resolver := netutil.NewCustomDNSResolver(addrs, 2*time.Minute)
//
//		dsn := "master.pgcluster11.consul"
//		connector, err := pq.NewConnector(dsn)
//		if err != nil {
//			return nil, fmt.Errorf("new connector: %w", err)
//		}
//
//		connector.Dialer(&net.Dialer{Resolver: &resolver})
//
//		dbConn = sql.OpenDB(connector)
//
//		if err := dbConn.Ping(); err != nil {
//			panic(err)
//		}
//	}
func NewCustomDNSResolver(addrs []string, timeout time.Duration) net.Resolver {
	var (
		idx      = uint32(0)
		addrsLen = uint32(len(addrs)) //nolint:gosec // address count is reasonable
	)

	return net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: timeout}

			addr := addrs[atomic.AddUint32(&idx, 1)%addrsLen]

			return d.DialContext(ctx, "udp", addr)
		},
	}
}
