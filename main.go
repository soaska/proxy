package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"syscall"

	"github.com/c-robinson/iplib"
	"golang.org/x/sys/unix"

	"github.com/soaska/proxy/internal/socks5"

	"github.com/soaska/proxy/internal/api"
	"github.com/soaska/proxy/internal/bot"
	"github.com/soaska/proxy/internal/database"
	"github.com/soaska/proxy/internal/geoip"
	"github.com/soaska/proxy/internal/speedtest"
	"github.com/soaska/proxy/internal/stats"
)

func main() {
	// Load configuration
	if err := loadConfig(); err != nil {
		panic(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start whitelist update loop
	go checkIPsLoop()

	// Initialize statistics if enabled
	var statsCollector *stats.StatsCollector
	var geoipService *geoip.Service
	var speedtestService *speedtest.Service

	if cfg.Stats.Enabled {
		log.Println("[STATS] Initializing statistics collection...")

		// Initialize database
		db, err := database.InitDB(cfg.Stats.DatabasePath)
		if err != nil {
			log.Printf("[STATS] Failed to initialize database: %v", err)
		} else {
			// Initialize GeoIP
			geoipService, err = geoip.NewService(cfg.Stats.GeoIPPath)
			if err != nil {
				log.Printf("[STATS] Failed to initialize GeoIP: %v", err)
				log.Printf("[STATS] Continuing without GeoIP support")
			}

			// Initialize stats collector
			statsCollector = stats.NewStatsCollector(db, geoipService, cfg.Stats.RetentionDays)

			// Initialize speedtest service
			speedtestService = speedtest.NewService(db, geoipService)

			log.Println("[STATS] Statistics collection initialized")
		}
	}

	// Start HTTP API server if enabled
	if cfg.API.Enabled && statsCollector != nil {
		apiServer := api.NewServer(statsCollector, speedtestService, cfg.API.APIKey, cfg.API.CORSOrigins)
		go func() {
			if err := apiServer.Start(ctx, cfg.API.Listen); err != nil {
				log.Printf("[API] Server error: %v", err)
			}
		}()
	}

	// Start Telegram bot if enabled
	if cfg.Telegram.Enabled && cfg.Telegram.BotToken != "" {
		telegramBot, err := bot.NewBot(cfg.Telegram.BotToken, cfg.Telegram.AdminIDs, statsCollector, speedtestService)
		if err != nil {
			log.Printf("[BOT] Failed to initialize bot: %v", err)
		} else {
			go func() {
				if err := telegramBot.Start(ctx); err != nil {
					log.Printf("[BOT] Bot error: %v", err)
				}
			}()
		}
	}

	// Setup SOCKS5 server
	subnet := iplib.NewNet4(net.ParseIP(cfg.Subnet), cfg.SubnetMask)

	server := &socks5.Server{
		Dialer: func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("failed to split host and port: %w", err)
			}

			ip, err := net.ResolveIPAddr("ip4", host)
			if err != nil {
				return nil, fmt.Errorf("failed to resolve IP: %w", err)
			}

			// Check whitelist
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

			// Valid destination
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
				}

				// Set appropriate local address based on network type
				if network == "tcp" || network == "tcp4" || network == "tcp6" {
					dialer.LocalAddr = &net.TCPAddr{
						IP: newAddr,
					}
					network = "tcp4"
				} else if network == "udp" || network == "udp4" || network == "udp6" {
					dialer.LocalAddr = &net.UDPAddr{
						IP: newAddr,
					}
					network = "udp4"
				}

				conn, err := dialer.DialContext(dialCtx, network, addr)
				if err != nil {
					log.Println("Failed to dial:", err)
					return nil, err
				}

				// Track connection if stats enabled
				if statsCollector != nil {
					clientIP := socks5.ClientAddr(dialCtx)
					if clientIP != "" {
						if host, _, err := net.SplitHostPort(clientIP); err == nil {
							clientIP = host
						}
					}
					if clientIP == "" {
						clientIP = "unknown"
					}
					tracker := statsCollector.TrackConnection(dialCtx, clientIP, addr)
					if tracker != nil {
						// Wrap connection with tracker
						conn = tracker.WrapConnection(conn)
						// Setup cleanup on connection close
						go func() {
							<-dialCtx.Done()
							tracker.Close(context.Background())
						}()
					}
				}

				return conn, nil
			}

			return nil, fmt.Errorf("ip %s is not in the whitelist", ip.IP.String())
		},
	}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		panic(err)
	}

	log.Printf("SOCKS5 proxy server started on %s", ln.Addr().String())

	// Start server in goroutine
	go func() {
		if err := server.Serve(ln); err != nil {
			log.Printf("Server error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	<-sigChan
	log.Println("Shutting down gracefully...")
	cancel()
	ln.Close()

	// Close stats collector if initialized
	if statsCollector != nil {
		statsCollector.Close()
	}

	log.Println("Shutdown complete")
}
