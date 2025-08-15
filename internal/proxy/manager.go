package proxy

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"proxy-v6/pkg/models"
	"github.com/sirupsen/logrus"
)

type Manager struct {
	logger      *logrus.Logger
	instances   map[string]*models.ProxyInstance
	mu          sync.RWMutex
	startPort   int
	endPort     int
	currentPort int
	processes   map[string]*exec.Cmd
	allowedIPs  []string
	proxyMode   string
}

func NewManager(logger *logrus.Logger, startPort, endPort int) *Manager {
	return &Manager{
		logger:      logger,
		instances:   make(map[string]*models.ProxyInstance),
		startPort:   startPort,
		endPort:     endPort,
		currentPort: startPort,
		processes:   make(map[string]*exec.Cmd),
		allowedIPs:  []string{},
		proxyMode:   "open",
	}
}

func (m *Manager) SetAccessControl(allowedIPs []string, mode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.allowedIPs = allowedIPs
	m.proxyMode = mode
	m.logger.Infof("Proxy access control set to mode: %s with %d allowed IPs", mode, len(allowedIPs))
}

func (m *Manager) StartProxy(ctx context.Context, ipv6 models.IPv6Address) (*models.ProxyInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	port := m.getNextPort()
	if port == 0 {
		return nil, fmt.Errorf("no available ports")
	}
	
	instanceID := fmt.Sprintf("%s-%d", ipv6.IP.String(), port)
	m.logger.Debugf("Starting proxy instance: %s", instanceID)
	
	configPath := fmt.Sprintf("/tmp/tinyproxy-%s.conf", instanceID)
	if err := m.createTinyproxyConfig(configPath, ipv6.IP.String(), port); err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
	}
	m.logger.Debugf("Created config file: %s", configPath)
	
	// Add debug mode and foreground mode for better error visibility
	cmd := exec.CommandContext(ctx, "tinyproxy", "-d", "-c", configPath)
	
	// Capture stdout and stderr for debugging
	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()
	
	if err := cmd.Start(); err != nil {
		m.logger.Errorf("Failed to start tinyproxy for %s: %v", instanceID, err)
		// Try to read any output that might have been produced
		if output, _ := os.ReadFile(configPath); len(output) > 0 {
			m.logger.Debugf("Config file contents:\n%s", string(output))
		}
		return nil, fmt.Errorf("failed to start tinyproxy: %w", err)
	}
	
	// Start goroutines to capture output
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stdoutPipe.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				m.logger.Infof("Tinyproxy[%s] stdout: %s", instanceID, string(buf[:n]))
			}
		}
	}()
	
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := stderrPipe.Read(buf)
			if err != nil {
				break
			}
			if n > 0 {
				m.logger.Warnf("Tinyproxy[%s] stderr: %s", instanceID, string(buf[:n]))
			}
		}
	}()
	
	instance := &models.ProxyInstance{
		ID:        instanceID,
		IPv6:      ipv6,
		Port:      port,
		Status:    models.ProxyStatusStarting,
		StartedAt: time.Now(),
		LastChecked: time.Now(),
		Metrics:   models.ProxyMetrics{},
	}
	
	m.instances[instanceID] = instance
	m.processes[instanceID] = cmd
	
	go m.monitorProcess(instanceID, cmd)
	
	// Give tinyproxy more time to start up and check multiple times
	retries := 5
	for i := 0; i < retries; i++ {
		time.Sleep(2 * time.Second)
		
		// Check if process is still running
		if cmd.Process != nil {
			// Use kill -0 to check if process exists
			if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
				m.logger.Errorf("Tinyproxy process died during startup (attempt %d/%d): %v", i+1, retries, err)
				// Try to get exit status
				if cmd.ProcessState != nil {
					m.logger.Errorf("Process exit code: %d", cmd.ProcessState.ExitCode())
				}
				// Read log file for errors
				if logContent, err := os.ReadFile(fmt.Sprintf("/tmp/tinyproxy-%s-%d.log", ipv6.IP.String(), port)); err == nil && len(logContent) > 0 {
					m.logger.Errorf("Tinyproxy log contents:\n%s", string(logContent))
				}
				instance.Status = models.ProxyStatusError
				return instance, fmt.Errorf("tinyproxy process died during startup")
			}
		}
		
		if m.checkProxyHealth(ipv6.IP.String(), port) {
			instance.Status = models.ProxyStatusRunning
			m.logger.Infof("Proxy started successfully: %s on port %d (attempt %d/%d)", ipv6.IP.String(), port, i+1, retries)
			break
		} else if i == retries-1 {
			instance.Status = models.ProxyStatusError
			m.logger.Errorf("Proxy failed health check after %d attempts: %s on port %d", retries, ipv6.IP.String(), port)
			// Read log file for debugging
			if logContent, err := os.ReadFile(fmt.Sprintf("/tmp/tinyproxy-%s-%d.log", ipv6.IP.String(), port)); err == nil && len(logContent) > 0 {
				m.logger.Errorf("Tinyproxy log contents:\n%s", string(logContent))
			}
		} else {
			m.logger.Debugf("Proxy not ready yet, retrying... (attempt %d/%d)", i+1, retries)
		}
	}
	
	return instance, nil
}

func (m *Manager) StopProxy(instanceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	instance, exists := m.instances[instanceID]
	if !exists {
		return fmt.Errorf("proxy instance not found: %s", instanceID)
	}
	
	if cmd, ok := m.processes[instanceID]; ok {
		if err := cmd.Process.Kill(); err != nil {
			m.logger.Warnf("Failed to kill process for %s: %v", instanceID, err)
		}
		delete(m.processes, instanceID)
	}
	
	instance.Status = models.ProxyStatusStopped
	m.logger.Infof("Proxy stopped: %s", instanceID)
	
	return nil
}

func (m *Manager) GetInstances() []models.ProxyInstance {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	instances := make([]models.ProxyInstance, 0, len(m.instances))
	for _, instance := range m.instances {
		instances = append(instances, *instance)
	}
	return instances
}

func (m *Manager) UpdateMetrics(instanceID string, metrics models.ProxyMetrics) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if instance, exists := m.instances[instanceID]; exists {
		instance.Metrics = metrics
		instance.LastChecked = time.Now()
	}
}

func (m *Manager) getNextPort() int {
	for i := m.currentPort; i <= m.endPort; i++ {
		portInUse := false
		for _, instance := range m.instances {
			if instance.Port == i && instance.Status == models.ProxyStatusRunning {
				portInUse = true
				break
			}
		}
		if !portInUse {
			m.currentPort = i + 1
			return i
		}
	}
	
	for i := m.startPort; i < m.currentPort; i++ {
		portInUse := false
		for _, instance := range m.instances {
			if instance.Port == i && instance.Status == models.ProxyStatusRunning {
				portInUse = true
				break
			}
		}
		if !portInUse {
			m.currentPort = i + 1
			return i
		}
	}
	
	return 0
}

func (m *Manager) createTinyproxyConfig(path, bindIP string, port int) error {
	// Build Allow directives based on access control mode
	allowDirectives := ""
	
	// Always allow localhost connections for health checks
	allowDirectives += "Allow 127.0.0.1\n"
	allowDirectives += "Allow ::1\n"
	
	// Also allow connections from the same IPv6 address (for health checks)
	allowDirectives += fmt.Sprintf("Allow %s\n", bindIP)
	
	if m.proxyMode == "restricted" && len(m.allowedIPs) > 0 {
		// In restricted mode, only allow specified IPs
		for _, ip := range m.allowedIPs {
			allowDirectives += fmt.Sprintf("Allow %s\n", ip)
		}
	} else if m.proxyMode == "open" {
		// In open mode, allow all (use with caution!)
		allowDirectives += "Allow 0.0.0.0/0\nAllow ::/0"
	}
	// If restricted mode but no IPs, only localhost and bindIP are allowed
	
	config := fmt.Sprintf(`# Basic Configuration
Port %d
Listen %s

# Server Configuration  
MaxClients 100
MinSpareServers 5
MaxSpareServers 20
StartServers 10
MaxRequestsPerChild 10000

# Access Control
%s

# Logging
LogLevel Info
LogFile "/tmp/tinyproxy-%s-%d.log"
PidFile "/tmp/tinyproxy-%s-%d.pid"

# Proxy Configuration
ViaProxyName "proxy-v6"
DisableViaHeader No
Timeout 600

# Performance
ConnectPort 443
ConnectPort 563
ConnectPort 993
ConnectPort 995
ConnectPort 80
ConnectPort 8080
ConnectPort 8443
`, port, bindIP, allowDirectives, bindIP, port, bindIP, port)
	
	return os.WriteFile(path, []byte(config), 0644)
}

func (m *Manager) checkProxyHealth(ip string, port int) bool {
	// Simple TCP connection test to avoid generating errors in tinyproxy logs
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("[%s]:%d", ip, port), 3*time.Second)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func (m *Manager) monitorProcess(instanceID string, cmd *exec.Cmd) {
	if err := cmd.Wait(); err != nil {
		m.logger.Warnf("Process exited with error for %s: %v", instanceID, err)
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if instance, exists := m.instances[instanceID]; exists {
		if instance.Status == models.ProxyStatusRunning {
			instance.Status = models.ProxyStatusError
			m.logger.Errorf("Proxy process died unexpectedly: %s", instanceID)
		}
	}
	
	delete(m.processes, instanceID)
}