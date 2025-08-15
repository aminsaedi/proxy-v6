package models

import (
	"net"
	"time"
)

type IPv6Address struct {
	IP        net.IP    `json:"ip"`
	Interface string    `json:"interface"`
	IsPublic  bool      `json:"is_public"`
	CreatedAt time.Time `json:"created_at"`
}

type ProxyInstance struct {
	ID          string      `json:"id"`
	IPv6        IPv6Address `json:"ipv6"`
	Port        int         `json:"port"`
	Status      ProxyStatus `json:"status"`
	StartedAt   time.Time   `json:"started_at"`
	LastChecked time.Time   `json:"last_checked"`
	Metrics     ProxyMetrics `json:"metrics"`
}

type ProxyStatus string

const (
	ProxyStatusStarting ProxyStatus = "starting"
	ProxyStatusRunning  ProxyStatus = "running"
	ProxyStatusStopped  ProxyStatus = "stopped"
	ProxyStatusError    ProxyStatus = "error"
)

type ProxyMetrics struct {
	RequestsTotal   int64     `json:"requests_total"`
	BytesTransmitted int64    `json:"bytes_transmitted"`
	ErrorCount      int64     `json:"error_count"`
	LastRequest     time.Time `json:"last_request"`
	ResponseTime    float64   `json:"response_time_ms"`
}

type NodeInfo struct {
	NodeID    string          `json:"node_id"`
	Hostname  string          `json:"hostname"`
	Region    string          `json:"region"`
	Proxies   []ProxyInstance `json:"proxies"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type Config struct {
	Mode           string        `json:"mode"` // "agent" or "coordinator"
	AgentConfig    AgentConfig   `json:"agent_config,omitempty"`
	CoordinatorConfig CoordinatorConfig `json:"coordinator_config,omitempty"`
}

type AgentConfig struct {
	ListenPort      int      `json:"listen_port"`
	ProxyStartPort  int      `json:"proxy_start_port"`
	ProxyEndPort    int      `json:"proxy_end_port"`
	CoordinatorURL  string   `json:"coordinator_url"`
	MetricsPort     int      `json:"metrics_port"`
	ExcludeInterfaces []string `json:"exclude_interfaces"`
	AllowedIPs      []string `json:"allowed_ips"`      // IPs allowed to connect to proxies
	ProxyMode       string   `json:"proxy_mode"`       // "open" or "restricted"
}

type CoordinatorConfig struct {
	ListenPort     int      `json:"listen_port"`
	ProxyPort      int      `json:"proxy_port"`
	MetricsPort    int      `json:"metrics_port"`
	AgentEndpoints []string `json:"agent_endpoints"`
	HealthCheckInterval time.Duration `json:"health_check_interval"`
}