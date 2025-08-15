package loadbalancer

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"proxy-v6/pkg/models"
	"github.com/sirupsen/logrus"
)

type LoadBalancer struct {
	logger      *logrus.Logger
	proxies     []ProxyEndpoint
	mu          sync.RWMutex
	roundRobin  uint64
	httpClient  *http.Client
	healthCheck *HealthChecker
}

type ProxyEndpoint struct {
	NodeID    string
	Address   string
	Healthy   bool
	LastCheck time.Time
}

type HealthChecker struct {
	interval time.Duration
	timeout  time.Duration
	logger   *logrus.Logger
}

func NewLoadBalancer(logger *logrus.Logger, checkInterval time.Duration) *LoadBalancer {
	lb := &LoadBalancer{
		logger:  logger,
		proxies: make([]ProxyEndpoint, 0),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		healthCheck: &HealthChecker{
			interval: checkInterval,
			timeout:  5 * time.Second,
			logger:   logger,
		},
	}
	
	go lb.startHealthChecks()
	return lb
}

func (lb *LoadBalancer) UpdateProxies(nodes []models.NodeInfo) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	newProxies := make([]ProxyEndpoint, 0)
	
	for _, node := range nodes {
		for _, proxy := range node.Proxies {
			if proxy.Status == models.ProxyStatusRunning {
				endpoint := ProxyEndpoint{
					NodeID:    node.NodeID,
					Address:   fmt.Sprintf("[%s]:%d", proxy.IPv6.IP.String(), proxy.Port),
					Healthy:   true,
					LastCheck: time.Now(),
				}
				newProxies = append(newProxies, endpoint)
			}
		}
	}
	
	lb.proxies = newProxies
	lb.logger.Infof("Updated proxy pool: %d endpoints", len(newProxies))
}

func (lb *LoadBalancer) GetNextProxy() (*ProxyEndpoint, error) {
	lb.mu.RLock()
	defer lb.mu.RUnlock()
	
	if len(lb.proxies) == 0 {
		return nil, fmt.Errorf("no proxies available")
	}
	
	healthyProxies := make([]ProxyEndpoint, 0)
	for _, p := range lb.proxies {
		if p.Healthy {
			healthyProxies = append(healthyProxies, p)
		}
	}
	
	if len(healthyProxies) == 0 {
		return nil, fmt.Errorf("no healthy proxies available")
	}
	
	index := atomic.AddUint64(&lb.roundRobin, 1) % uint64(len(healthyProxies))
	return &healthyProxies[index], nil
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	proxy, err := lb.GetNextProxy()
	if err != nil {
		http.Error(w, "No proxy available", http.StatusServiceUnavailable)
		return
	}
	
	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", proxy.Address))
	
	proxyReq, err := http.NewRequest(r.Method, r.URL.String(), r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}
	
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}
	
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	
	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}
	
	resp, err := client.Do(proxyReq)
	if err != nil {
		lb.markProxyUnhealthy(proxy.Address)
		http.Error(w, "Proxy request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		lb.logger.Warnf("Error copying response body: %v", err)
	}
}

func (lb *LoadBalancer) startHealthChecks() {
	ticker := time.NewTicker(lb.healthCheck.interval)
	defer ticker.Stop()
	
	for range ticker.C {
		lb.performHealthChecks()
	}
}

func (lb *LoadBalancer) performHealthChecks() {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	for i := range lb.proxies {
		go lb.checkProxyHealth(&lb.proxies[i])
	}
}

func (lb *LoadBalancer) checkProxyHealth(proxy *ProxyEndpoint) {
	conn, err := net.DialTimeout("tcp", proxy.Address, lb.healthCheck.timeout)
	if err != nil {
		proxy.Healthy = false
		lb.healthCheck.logger.Warnf("Proxy %s failed health check: %v", proxy.Address, err)
	} else {
		conn.Close()
		proxy.Healthy = true
	}
	proxy.LastCheck = time.Now()
}

func (lb *LoadBalancer) markProxyUnhealthy(address string) {
	lb.mu.Lock()
	defer lb.mu.Unlock()
	
	for i := range lb.proxies {
		if lb.proxies[i].Address == address {
			lb.proxies[i].Healthy = false
			lb.logger.Warnf("Marked proxy %s as unhealthy", address)
			break
		}
	}
}