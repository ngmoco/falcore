package upstream

import (
	"os"
	"strings"
	"strconv"
	"sync"
	"time"
	"falcore"
	"http"
)

type UpstreamEntryConfig struct {
	HostPort  string
	Weight    int
	ForceHttp bool
}

type UpstreamEntry struct {
	Upstream *Upstream
	Weight   int
}
// An UpstreamPool is a list of upstream servers which are considered
// functionally equivalent.  The pool will round-robin the requests to the servers.
type UpstreamPool struct {
	pool         []*UpstreamEntry
	rr_count     int
	ping_count   int64
	Name         string
	nextUpstream chan *UpstreamEntry
	shutdown     chan int
	weightMutex  *sync.RWMutex
	pinger       *time.Ticker
}

// The config consists of a map of the servers in the pool in the format host_or_ip:port 
// where port is optional and defaults to 80.  The map value is an int with the weight
// only 0 and 1 are supported weights (0 disables a server and 1 enables it)
func NewUpstreamPool(name string, config []UpstreamEntryConfig) *UpstreamPool {
	up := new(UpstreamPool)
	up.pool = make([]*UpstreamEntry, len(config))
	up.Name = name
	up.nextUpstream = make(chan *UpstreamEntry)
	up.weightMutex = new(sync.RWMutex)
	up.shutdown = make(chan int)
	up.pinger = time.NewTicker(1e9) // 1s

	// create the pool
	for i, uec := range config {
		parts := strings.Split(uec.HostPort, ":")
		upstreamHost := parts[0]
		upstreamPort := 80
		if len(parts) > 1 {
			var err os.Error
			upstreamPort, err = strconv.Atoi(parts[1])
			if err != nil {
				upstreamPort = 80
				falcore.Error("UpstreamPool Error converting port to int for", upstreamHost, ":", err)
			}
		}
		ups := NewUpstream(upstreamHost, upstreamPort, uec.ForceHttp)
		ue := new(UpstreamEntry)
		ue.Upstream = ups
		ue.Weight = uec.Weight
		up.pool[i] = ue
	}
	go up.nextServer()
	return up
}

func (up UpstreamPool) Next() *Upstream {
	// TODO check in case all are down that we timeout
	return (<-up.nextUpstream).Upstream
}

// do we have > thresh upstreams available
func (up UpstreamPool) Status(thresh int) bool {
	sum := 0
	defer up.weightMutex.RUnlock()
	up.weightMutex.RLock()
	for _, ue := range up.pool {
		sum += ue.Weight
		if sum > thresh {
			return true
		}
	}
	return false
}

func (up UpstreamPool) FilterRequest(req *falcore.Request) *http.Response {
	return up.Next().FilterRequest(req)
}

// This should only be called if the upstream pool is no longer active or this may deadlock
func (up UpstreamPool) Shutdown() {
	// ping and nextServer
	close(up.shutdown)

	// make sure we hit the shutdown code in the nextServer goroutine
	up.Next()
}

func (up UpstreamPool) nextServer() {
	loopCount := 0
	for {
		next := up.rr_count % len(up.pool)
		up.weightMutex.RLock()
		wgt := up.pool[next].Weight
		up.weightMutex.RUnlock()
		// just return a down host if we've gone through the list twice and nothing is up
		// be sure to never return negative wgt hosts
		if (wgt > 0 || (loopCount > 2*len(up.pool))) && wgt >= 0 {
			loopCount = 0
			select {
			case <-up.shutdown:
				return
			case up.nextUpstream <- up.pool[next]:
			}
		} else {
			loopCount++
		}
		up.rr_count++
	}
}
