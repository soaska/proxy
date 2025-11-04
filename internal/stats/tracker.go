package stats

import (
	"context"
	"log"
	"net"
	"sync/atomic"
	"time"
)

// ConnectionTracker tracks statistics for a single connection
type ConnectionTracker struct {
	id        uint64
	collector *StatsCollector
	country   string
	bytesIn   atomic.Int64
	bytesOut  atomic.Int64
	startTime time.Time
	closed    atomic.Bool
}

// WrapConnection wraps a net.Conn to track traffic
func (ct *ConnectionTracker) WrapConnection(conn net.Conn) net.Conn {
	return &trackedConn{
		Conn:    conn,
		tracker: ct,
	}
}

// AddBytesIn adds to the bytes in counter
func (ct *ConnectionTracker) AddBytesIn(n int64) {
	ct.bytesIn.Add(n)
}

// AddBytesOut adds to the bytes out counter
func (ct *ConnectionTracker) AddBytesOut(n int64) {
	ct.bytesOut.Add(n)
}

// Close finalizes the connection tracking
func (ct *ConnectionTracker) Close(ctx context.Context) {
	if !ct.closed.CompareAndSwap(false, true) {
		return
	}

	bytesIn := ct.bytesIn.Load()
	bytesOut := ct.bytesOut.Load()
	duration := int64(time.Since(ct.startTime).Seconds())
	totalBytes := bytesIn + bytesOut

	// Update database
	_, err := ct.collector.db.ExecContext(ctx,
		`UPDATE connections
		 SET bytes_in = ?, bytes_out = ?, disconnected_at = ?, duration = ?
		 WHERE id = ?`,
		bytesIn, bytesOut, time.Now(), duration, ct.id,
	)
	if err != nil {
		log.Printf("[STATS] Failed to update connection stats: %v", err)
	}

	// Update counters
	ct.collector.activeCount.Add(-1)
	ct.collector.updateServerStats(0, bytesIn, bytesOut)
	ct.collector.updateGeoStats(ct.country, totalBytes, false)
	ct.collector.activeConns.Delete(ct.id)

	log.Printf("[STATS] Connection closed: ID=%d, Duration=%ds, In=%d, Out=%d",
		ct.id, duration, bytesIn, bytesOut)
}

// trackedConn wraps a connection to track bytes transferred
type trackedConn struct {
	net.Conn
	tracker *ConnectionTracker
}

func (tc *trackedConn) Read(p []byte) (n int, err error) {
	n, err = tc.Conn.Read(p)
	if n > 0 {
		tc.tracker.AddBytesIn(int64(n))
	}
	return
}

func (tc *trackedConn) Write(p []byte) (n int, err error) {
	n, err = tc.Conn.Write(p)
	if n > 0 {
		tc.tracker.AddBytesOut(int64(n))
	}
	return
}
