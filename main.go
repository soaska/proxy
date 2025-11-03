package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"runtime"
	"strings"
	"syscall"

	"github.com/c-robinson/iplib"
	"golang.org/x/sys/unix"
	"tailscale.com/net/socks5"
)

func main() {
	if err := loadConfig(); err != nil {
		panic(err)
	}
	go checkIPsLoop()

	subnet := iplib.NewNet4(net.ParseIP(cfg.Subnet), cfg.SubnetMask)

	server := &socks5.Server{
		Dialer: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("failed to split host and port: %w", err)
			}

			ip, err := net.ResolveIPAddr("ip4", host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve IP: %w", err)
			}

			wlMutex.RLock()
			_, ok := whitelist[ip.String()]
			wlMutex.RUnlock()

			if !ok {
				ok = isIPInRange(ip.IP)
			}

			if !ok {
				for _, whost := range cfg.Whitelist {
					if strings.EqualFold(host, whost) {
						ok = true
						break
					}
				}
			}

			// Valid dest
			if ok {
				newAddr := subnet.RandomIP()
				log.Println("Dialing", network, addr, "from", newAddr)

				dialer := &net.Dialer{
					Control: func(network, address string, c syscall.RawConn) error {
						if runtime.GOOS != "linux" {
							return nil
						}
						var operr error
						if err := c.Control(func(fd uintptr) {
							const IP_FREEBIND = 15
							operr = unix.SetsockoptInt(int(fd), unix.IPPROTO_IP, IP_FREEBIND, 1)
						}); err != nil {
							return err
						}
						return operr
					},
					LocalAddr: &net.TCPAddr{
						IP: newAddr,
					},
				}

				conn, err := dialer.DialContext(ctx, "tcp4", addr)
				if err != nil {
					log.Println("Failed to dial:", err)
				}
				return conn, err
			}

			return nil, fmt.Errorf("ip %s is not in the whitelist", ip.IP.String())
		},
	}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		panic(err)
	}

	log.Printf("SOCKS5 proxy server started on %s", ln.Addr().String())

	if err := server.Serve(ln); err != nil {
		panic(err)
	}
}
