package proxy

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"time"

	"proxy-v6/pkg/models"
	"github.com/sirupsen/logrus"
)

type Manager struct {
	logger     *logrus.Logger
	instances  map[string]*models.ProxyInstance
	mu         sync.RWMutex
	startPort  int
	endPort    int
	currentPort int
	processes  map[string]*exec.Cmd
}

func NewManager(logger *logrus.Logger, startPort, endPort int) *Manager {
	return &Manager{
		logger:      logger,
		instances:   make(map[string]*models.ProxyInstance),
		startPort:   startPort,
		endPort:     endPort,
		currentPort: startPort,
		processes:   make(map[string]*exec.Cmd),
	}
}

func (m *Manager) StartProxy(ctx context.Context, ipv6 models.IPv6Address) (*models.ProxyInstance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	port := m.getNextPort()
	if port == 0 {
		return nil, fmt.Errorf("no available ports")
	}
	
	instanceID := fmt.Sprintf("%s-%d", ipv6.IP.String(), port)
	
	configPath := fmt.Sprintf("/tmp/tinyproxy-%s.conf", instanceID)
	if err := m.createTinyproxyConfig(configPath, ipv6.IP.String(), port); err != nil {
		return nil, fmt.Errorf("failed to create config: %w", err)
	}
	
	cmd := exec.CommandContext(ctx, "tinyproxy", "-c", configPath)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start tinyproxy: %w", err)
	}
	
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
	
	time.Sleep(2 * time.Second)
	
	if m.checkProxyHealth(ipv6.IP.String(), port) {
		instance.Status = models.ProxyStatusRunning
		m.logger.Infof("Proxy started successfully: %s on port %d", ipv6.IP.String(), port)
	} else {
		instance.Status = models.ProxyStatusError
		m.logger.Errorf("Proxy failed health check: %s on port %d", ipv6.IP.String(), port)
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
	config := fmt.Sprintf(`
Port %d
Listen %s
MaxClients 100
MinSpareServers 5
MaxSpareServers 20
StartServers 10
MaxRequestsPerChild 0
Allow 0.0.0.0/0
Allow ::/0
ViaProxyName "tinyproxy"
DisableViaHeader No
LogLevel Info
LogFile "/tmp/tinyproxy-%s-%d.log"
PidFile "/tmp/tinyproxy-%s-%d.pid"
`, port, bindIP, bindIP, port, bindIP, port)
	
	return os.WriteFile(path, []byte(config), 0644)
}

func (m *Manager) checkProxyHealth(ip string, port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("[%s]:%d", ip, port), 5*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

func (m *Manager) monitorProcess(instanceID string, cmd *exec.Cmd) {
	cmd.Wait()
	
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