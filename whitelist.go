package main

import (
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

var (
	whitelist  = map[string]struct{}{}
	ipRanges   = []*net.IPNet{}
	wlMutex    = sync.RWMutex{}
	rangeMutex = sync.RWMutex{}
)

func checkHostIPs(wg *sync.WaitGroup, host string) {
	defer wg.Done()

	if strings.Contains(host, "/") {
		_, ipNet, err := net.ParseCIDR(host)
		if err != nil {
			log.Printf("Failed to parse CIDR %s: %v", host, err)
			return
		}

		rangeMutex.Lock()
		ipRanges = append(ipRanges, ipNet)
		rangeMutex.Unlock()

		log.Printf("Added IP range: %s", host)
		return
	}

	// Resolve the host to get the IPs
	ips, err := net.LookupHost(host)
	if err != nil {
		log.Printf("Failed to resolve host %s: %v", host, err)
		return
	}

	// No IPs found
	if len(ips) == 0 {
		log.Println("No IPs found for host", host)
		return
	}

	// Update the whitelist
	hostWhitelist := make([]net.IP, 0, len(ips))
	for _, ipStr := range ips {
		ip, err := net.ResolveIPAddr("ip", ipStr)
		if err != nil {
			log.Println("Failed to resolve IP:", err)
			continue
		}
		hostWhitelist = append(hostWhitelist, ip.IP)

		wlMutex.Lock()
		whitelist[ip.String()] = struct{}{}
		wlMutex.Unlock()
	}

	log.Println("Resolved", host, "to", hostWhitelist)
}

func checkIPs() {
	rangeMutex.Lock()
	ipRanges = []*net.IPNet{}
	rangeMutex.Unlock()

	wg := &sync.WaitGroup{}
	for _, host := range cfg.Whitelist {
		wg.Add(1)
		go checkHostIPs(wg, host)
	}
	wg.Wait()

	printWhitelist()
}

func printWhitelist() {
	wlMutex.RLock()
	ipCount := len(whitelist)
	wlMutex.RUnlock()

	rangeMutex.RLock()
	rangeCount := len(ipRanges)
	rangeMutex.RUnlock()

	log.Printf("Whitelist summary: %d resolved IPs, %d IP ranges", ipCount, rangeCount)

	if rangeCount > 0 {
		rangeMutex.RLock()
		log.Println("IP ranges:")
		for _, ipNet := range ipRanges {
			log.Printf("  - %s", ipNet.String())
		}
		rangeMutex.RUnlock()
	}

	if ipCount > 0 {
		wlMutex.RLock()
		log.Println("Resolved IPs:")
		for ip := range whitelist {
			log.Printf("  - %s", ip)
		}
		wlMutex.RUnlock()
	}
}

func isIPInRange(ip net.IP) bool {
	rangeMutex.RLock()
	defer rangeMutex.RUnlock()

	for _, ipNet := range ipRanges {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func checkIPsLoop() {
	checkIPs()

	ticker := time.NewTicker(cfg.UpdateInterval)
	for range ticker.C {
		checkIPs()
	}
}
