package checker

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
	"github.com/you/linkwatch/internal/store"
)

// Checker manages background URL checking.
type Checker struct {
	store           store.Store       // Database store
	checkInterval   time.Duration     // How often to check all targets
	maxConcurrency  int               // Max checks running in parallel
	httpTimeout     time.Duration     // Timeout for each HTTP request
	shutdownGrace   time.Duration     // How long to wait before forced shutdown

	workers         chan struct{}     // Semaphore for global concurrency
	hostSemaphores  map[string]chan struct{} // Per-host semaphores
	hostMutex       sync.RWMutex

	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

// NewChecker creates a new URL checker.
func NewChecker(
	store store.Store,
	checkInterval, httpTimeout, shutdownGrace time.Duration,
	maxConcurrency int,
) *Checker {
	ctx, cancel := context.WithCancel(context.Background())

	return &Checker{
		store:          store,
		checkInterval:  checkInterval,
		maxConcurrency: maxConcurrency,
		httpTimeout:    httpTimeout,
		shutdownGrace:  shutdownGrace,
		workers:        make(chan struct{}, maxConcurrency),
		hostSemaphores: make(map[string]chan struct{}),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins the background scheduler.
func (c *Checker) Start() {
	c.wg.Add(1)
	go c.scheduler()
}

// scheduler runs the main loop on a fixed interval.
func (c *Checker) scheduler() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.scheduleChecks()
		}
	}
}

// scheduleChecks fetches all targets and schedules checks for them.
func (c *Checker) scheduleChecks() {
	targets, _, err := c.store.GetTargets(c.ctx, "", time.Time{}, "", 1000)
	if err != nil {
		fmt.Println("failed to fetch targets:", err)
		return
	}

	for _, target := range targets {
		select {
		case <-c.ctx.Done():
			return
		case c.workers <- struct{}{}:
			c.wg.Add(1)
			go c.checkTarget(target)
		}
	}
}

// checkTarget performs a single URL check and stores the result.
func (c *Checker) checkTarget(target *store.Target) {
	defer c.wg.Done()
	defer func() { <-c.workers }()

	// Limit concurrent checks per host
	if !c.acquireHostSemaphore(target.Host) {
		return
	}
	defer c.releaseHostSemaphore(target.Host)

	// Perform HTTP check
	result := c.performCheck(target)

	// Save result
	if err := c.store.InsertCheckResult(c.ctx, result); err != nil {
		fmt.Println("failed to save check result:", err)
	}
}

// acquireHostSemaphore prevents overwhelming a single host.
func (c *Checker) acquireHostSemaphore(host string) bool {
	c.hostMutex.Lock()
	sem, exists := c.hostSemaphores[host]
	if !exists {
		// Allow up to 2 checks per host in parallel (tunable)
		sem = make(chan struct{}, 2)
		c.hostSemaphores[host] = sem
	}
	c.hostMutex.Unlock()

	select {
	case sem <- struct{}{}:
		return true
	case <-c.ctx.Done():
		return false
	}
}

// releaseHostSemaphore frees a host "slot".
func (c *Checker) releaseHostSemaphore(host string) {
	c.hostMutex.RLock()
	sem, exists := c.hostSemaphores[host]
	c.hostMutex.RUnlock()

	if exists {
		<-sem
	}
}

// performCheck makes the HTTP GET request and records results.
func (c *Checker) performCheck(target *store.Target) *store.CheckResult {
	start := time.Now()
	client := http.Client{Timeout: c.httpTimeout}

	resp, err := client.Get(target.URL)
	latency := time.Since(start).Milliseconds()

	result := &store.CheckResult{
		TargetID:  target.ID,
		CheckedAt: time.Now(),
		LatencyMs: int(latency),
	}

	if err != nil {
		errMsg := err.Error()
		result.Error = &errMsg
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = &resp.StatusCode
	return result
}

// Shutdown gracefully stops the checker and waits for workers to finish.
func (c *Checker) Shutdown() {
	// Tell scheduler + workers to stop
	c.cancel()

	// Wait for everything to finish, but only up to shutdownGrace
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		fmt.Println("checker shut down gracefully")
	case <-time.After(c.shutdownGrace):
		fmt.Println("shutdown timed out, forcing exit")
	}
}
