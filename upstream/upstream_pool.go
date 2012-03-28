package upstream

import (
	"github.com/ngmoco/falcore"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type UpstreamEntryConfig struct {
	HostPort  string
	Weight    int
	ForceHttp bool
	PingPath  string
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
	up.pinger = time.NewTicker(3e9) // 3s

	// create the pool
	for i, uec := range config {
		parts := strings.Split(uec.HostPort, ":")
		upstreamHost := parts[0]
		upstreamPort := 80
		if len(parts) > 1 {
			var err error
			upstreamPort, err = strconv.Atoi(parts[1])
			if err != nil {
				upstreamPort = 80
				falcore.Error("UpstreamPool Error converting port to int for", upstreamHost, ":", err)
			}
		}
		ups := NewUpstream(upstreamHost, upstreamPort, uec.ForceHttp)
		ups.PingPath = uec.PingPath
		ue := new(UpstreamEntry)
		ue.Upstream = ups
		ue.Weight = uec.Weight
		up.pool[i] = ue
	}
	go up.nextServer()
	go up.pingUpstreams()
	return up
}

func (up UpstreamPool) Next() *UpstreamEntry {
	// TODO check in case all are down that we timeout
	return <-up.nextUpstream
}

func (up UpstreamPool) LogStatus() {
	weightsBuffer := make([]int, len(up.pool))
	// loop and save the weights so we don't lock for logging
	up.weightMutex.RLock()
	for i, ue := range up.pool {
		weightsBuffer[i] = ue.Weight
	}
	up.weightMutex.RUnlock()
	// Now do the logging
	for i, ue := range up.pool {
		falcore.Info("Upstream %v: %v:%v\t%v", up.Name, ue.Upstream.Host, ue.Upstream.Port, weightsBuffer[i])
	}
}

func (up UpstreamPool) FilterRequest(req *falcore.Request) (res *http.Response) {
	ue := up.Next()
	res = ue.Upstream.FilterRequest(req)
	if req.CurrentStage.Status == 2 {
		// this gets set by the upstream for errors
		// so mark this upstream as down
		up.updateUpstream(ue, 0)
		up.LogStatus()
	}
	return
}

func (up UpstreamPool) updateUpstream(ue *UpstreamEntry, wgt int) {
	up.weightMutex.Lock()
	ue.Weight = wgt
	up.weightMutex.Unlock()
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

func (up UpstreamPool) pingUpstreams() {
	pingable := true
	for pingable {
		select {
		case <-up.shutdown:
			return
		case <-up.pinger.C:
			gotone := false
			for i, ups := range up.pool {
				if ups.Upstream.PingPath != "" {
					go up.pingUpstream(ups, i)
					gotone = true
				}
			}
			if !gotone {
				pingable = false
			}
		}
	}
	falcore.Warn("Stopping ping for %v", up.Name)
}

func (up UpstreamPool) pingUpstream(ups *UpstreamEntry, index int) {
	isUp, ok := ups.Upstream.ping()
	up.weightMutex.RLock()
	wgt := ups.Weight
	up.weightMutex.RUnlock()
	// change in status
	if ok && (wgt > 0) != isUp {
		if isUp {
			up.updateUpstream(ups, 1)
		} else {
			up.updateUpstream(ups, 0)
		}
		up.LogStatus()
	}
}
