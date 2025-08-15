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
	
	// Get the current counter value and increment atomically
	currentIndex := atomic.AddUint64(&lb.roundRobin, 1) - 1
	index := currentIndex % uint64(len(healthyProxies))
	selectedProxy := &healthyProxies[index]
	
	// Log which proxy was selected and why
	lb.logger.Infof("Selected proxy %d of %d: %s (NodeID: %s, Round-robin counter: %d)", 
		index+1, len(healthyProxies), selectedProxy.Address, selectedProxy.NodeID, currentIndex)
	
	return selectedProxy, nil
}

func (lb *LoadBalancer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Log incoming request
	lb.logger.Debugf("Incoming proxy request: %s %s from %s", r.Method, r.URL.String(), r.RemoteAddr)
	
	proxy, err := lb.GetNextProxy()
	if err != nil {
		lb.logger.Errorf("Failed to get proxy: %v", err)
		http.Error(w, "No proxy available", http.StatusServiceUnavailable)
		return
	}
	
	// Log which proxy will handle this request
	lb.logger.Infof("Forwarding request to proxy: %s (Node: %s) for URL: %s", 
		proxy.Address, proxy.NodeID, r.URL.String())
	
	proxyURL, _ := url.Parse(fmt.Sprintf("http://%s", proxy.Address))
	
	// For HTTP proxy requests, we need to use the full URL
	targetURL := r.URL.String()
	if !r.URL.IsAbs() {
		// If it's a CONNECT request (HTTPS), handle it differently
		if r.Method == "CONNECT" {
			lb.handleConnect(w, r, proxy)
			return
		}
		// For relative URLs, construct the full URL
		targetURL = fmt.Sprintf("http://%s%s", r.Host, r.RequestURI)
	}
	
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		lb.logger.Errorf("Failed to create proxy request: %v", err)
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}
	
	// Copy headers
	for key, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}
	
	// Use the selected proxy
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
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse // Don't follow redirects
		},
	}
	
	resp, err := client.Do(proxyReq)
	if err != nil {
		lb.logger.Errorf("Proxy request failed for %s: %v", proxy.Address, err)
		lb.markProxyUnhealthy(proxy.Address)
		http.Error(w, "Proxy request failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	
	lb.logger.Debugf("Proxy response: %d from %s", resp.StatusCode, proxy.Address)
	
	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	
	// Write status code
	w.WriteHeader(resp.StatusCode)
	
	// Copy response body
	written, err := io.Copy(w, resp.Body)
	if err != nil {
		lb.logger.Warnf("Error copying response body: %v", err)
	} else {
		lb.logger.Debugf("Response sent: %d bytes", written)
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
	// Simple TCP connection test - don't send HTTP requests as it causes errors in tinyproxy logs
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

func (lb *LoadBalancer) handleConnect(w http.ResponseWriter, r *http.Request, proxy *ProxyEndpoint) {
	lb.logger.Infof("Handling CONNECT request to %s via proxy %s", r.Host, proxy.Address)
	
	// Connect to the upstream proxy
	proxyConn, err := net.DialTimeout("tcp", proxy.Address, 10*time.Second)
	if err != nil {
		lb.logger.Errorf("Failed to connect to proxy %s: %v", proxy.Address, err)
		http.Error(w, "Failed to connect to proxy", http.StatusBadGateway)
		return
	}
	defer proxyConn.Close()
	
	// Send CONNECT request to the proxy
	connectReq := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", r.Host, r.Host)
	if _, err := proxyConn.Write([]byte(connectReq)); err != nil {
		lb.logger.Errorf("Failed to send CONNECT to proxy: %v", err)
		http.Error(w, "Failed to send CONNECT request", http.StatusBadGateway)
		return
	}
	
	// Read the proxy's response
	buf := make([]byte, 1024)
	n, err := proxyConn.Read(buf)
	if err != nil {
		lb.logger.Errorf("Failed to read CONNECT response: %v", err)
		http.Error(w, "Failed to read CONNECT response", http.StatusBadGateway)
		return
	}
	
	// Check if the proxy accepted the CONNECT
	response := string(buf[:n])
	if !contains(response, "200") {
		lb.logger.Errorf("Proxy rejected CONNECT: %s", response)
		http.Error(w, "Proxy rejected CONNECT", http.StatusBadGateway)
		return
	}
	
	// Hijack the client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		lb.logger.Error("Cannot hijack connection")
		http.Error(w, "Cannot hijack connection", http.StatusInternalServerError)
		return
	}
	
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		lb.logger.Errorf("Failed to hijack connection: %v", err)
		http.Error(w, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()
	
	// Send 200 Connection Established to the client
	clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	
	// Start bidirectional copy
	errc := make(chan error, 2)
	go func() {
		_, err := io.Copy(proxyConn, clientConn)
		errc <- err
	}()
	go func() {
		_, err := io.Copy(clientConn, proxyConn)
		errc <- err
	}()
	
	// Wait for one side to close
	<-errc
	lb.logger.Debugf("CONNECT tunnel closed for %s via %s", r.Host, proxy.Address)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr || len(s) > len(substr) && contains(s[1:], substr)
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