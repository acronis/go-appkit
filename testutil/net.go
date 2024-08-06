/*
Copyright © 2024 Acronis International GmbH.

Released under MIT license.
*/

package testutil

import (
	"errors"
	"fmt"
	"net"
	"time"
)

// GetLocalFreeTCPPort returns free (not listening by somebody) TCP port on the 127.0.0.1 network interface.
func GetLocalFreeTCPPort() int {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		panic(err)
	}
	return port
}

// GetLocalAddrWithFreeTCPPort returns 127.0.0.1:<free-tcp-port> address.
func GetLocalAddrWithFreeTCPPort() string {
	return fmt.Sprintf("127.0.0.1:%d", GetLocalFreeTCPPort())
}

// WaitListeningServer waits until the server is ready to accept TCP connection on the passing address.
func WaitListeningServer(addr string, timeout time.Duration) error {
	return waitListeningServer("tcp", addr, timeout)
}

// WaitPortAndListeningServer waits until port is known and the server is ready to accept TCP connection on the passing
// address.
func WaitPortAndListeningServer(host string, getPort func() int, timeout time.Duration) (int, error) {
	port, err := waitPort(getPort, timeout)
	if err != nil {
		return 0, err
	}

	return port, waitListeningServer("tcp", fmt.Sprintf("%s:%d", host, port), timeout)
}

// WaitListeningServerWithUnixSocket waits
// until the server is ready to accept unix socket connection on the passing address.
func WaitListeningServerWithUnixSocket(unixSocketPath string, timeout time.Duration) error {
	return waitListeningServer("unix", unixSocketPath, timeout)
}

func waitListeningServer(network string, addr string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	for {
		if conn, err := net.DialTimeout(network, addr, time.Second); err == nil {
			return conn.Close()
		}
		select {
		case <-timer.C:
			return errors.New("waiting listening server timed out")
		default:
			time.Sleep(time.Millisecond * 10)
		}
	}
}

func waitPort(getPort func() int, timeout time.Duration) (int, error) {
	timer := time.NewTimer(timeout)
	for {
		port := getPort()
		if port > 0 {
			return port, nil
		}
		select {
		case <-timer.C:
			return 0, errors.New("waiting for listening port appears")
		default:
			time.Sleep(time.Millisecond * 10)
		}
	}
}
