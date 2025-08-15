# Proxy-v6: Distributed IPv6 Proxy System

A distributed proxy management system that automatically discovers and utilizes IPv6 addresses across multiple nodes to create a pool of proxy servers with load balancing capabilities.

## Features

- **Automatic IPv6 Discovery**: Scans and identifies all public IPv6 addresses on each node
- **Distributed Architecture**: Run agents on multiple DigitalOcean droplets or VMs
- **Centralized Coordination**: Single coordinator manages all proxy agents
- **Load Balancing**: Round-robin distribution across healthy proxies
- **Health Monitoring**: Automatic health checks and failover
- **Metrics & Observability**: Prometheus-compatible metrics endpoints
- **TUI Dashboard**: Real-time monitoring interface
- **Auto-recovery**: Automatic restart of failed proxy instances

## Architecture

```
┌─────────────────┐
│   Coordinator   │
│  (Single Node)  │
│                 │
│  - Load Balancer│
│  - Health Check │
│  - API Gateway  │
└────────┬────────┘
         │
    ┌────┴────┐
    │         │
┌────▼──┐ ┌───▼───┐
│Agent 1│ │Agent 2│ ... 
│       │ │       │
│IPv6-1 │ │IPv6-4 │
│IPv6-2 │ │IPv6-5 │
│IPv6-3 │ │IPv6-6 │
└───────┘ └───────┘
```

## Installation

### Prerequisites

- Go 1.21 or higher
- tinyproxy installed on agent nodes
- IPv6 connectivity on agent nodes

### Install tinyproxy

```bash
# macOS
brew install tinyproxy

# Ubuntu/Debian
sudo apt-get install tinyproxy

# CentOS/RHEL
sudo yum install tinyproxy
```

### Build from source

```bash
# Clone the repository
git clone https://github.com/yourusername/proxy-v6.git
cd proxy-v6

# Install dependencies
make deps

# Build all binaries
make build
```

## Usage

### 1. Start Coordinator (on main server)

```bash
./bin/coordinator \
  --port 8081 \
  --proxy-port 8888 \
  --metrics-port 9091
```

### 2. Start Agents (on each node with IPv6)

```bash
./bin/agent \
  --port 8080 \
  --coordinator http://coordinator-ip:8081 \
  --proxy-start 10000 \
  --proxy-end 20000 \
  --metrics-port 9090
```

### 3. Monitor the System

```bash
./bin/monitor --coordinator http://coordinator-ip:8081
```

### 4. Use the Proxy

Configure your HTTP client to use the proxy:

```bash
# Using curl
curl -x http://coordinator-ip:8888 https://ipv6.google.com

# Using environment variables
export HTTP_PROXY=http://coordinator-ip:8888
export HTTPS_PROXY=http://coordinator-ip:8888
```

## Configuration

### Agent Configuration

```yaml
# agent-config.yaml
listen_port: 8080
proxy_start_port: 10000
proxy_end_port: 20000
coordinator_url: http://coordinator:8081
metrics_port: 9090
exclude_interfaces:
  - docker
  - veth
  - br-
```

### Coordinator Configuration

```yaml
# coordinator-config.yaml
listen_port: 8081
proxy_port: 8888
metrics_port: 9091
health_check_interval: 30s
```

## API Endpoints

### Coordinator API

- `GET /health` - Health check
- `GET /api/nodes` - List all registered nodes
- `GET /api/stats` - System statistics
- `POST /api/nodes/:nodeId` - Register/update node (used by agents)

### Agent API

- `GET /health` - Health check
- `GET /proxies` - List all proxy instances
- `GET /status` - Node status and proxy information
- `POST /proxy/:id/stop` - Stop a specific proxy instance

### Metrics

Both coordinator and agents expose Prometheus metrics:

- Agent: `http://agent-ip:9090/metrics`
- Coordinator: `http://coordinator-ip:9091/metrics`

## Deployment on DigitalOcean

### 1. Create Droplets with IPv6

```bash
# Create droplets with IPv6 enabled
doctl compute droplet create \
  --image ubuntu-22-04-x64 \
  --size s-1vcpu-1gb \
  --region nyc3 \
  --enable-ipv6 \
  --ssh-keys YOUR_SSH_KEY_ID \
  proxy-agent-1 proxy-agent-2 proxy-agent-3
```

### 2. Setup Agent on Each Droplet

```bash
# SSH into each droplet
ssh root@droplet-ip

# Install dependencies
apt update && apt install -y tinyproxy golang-go git

# Clone and build
git clone https://github.com/yourusername/proxy-v6.git
cd proxy-v6
make deps build-agent

# Run agent
./bin/agent --coordinator http://coordinator-ip:8081
```

### 3. Setup Coordinator

```bash
# On coordinator server
./bin/coordinator --proxy-port 8888
```

## Monitoring

The TUI monitor provides real-time visibility into:

- Number of active nodes
- Total proxy instances
- Healthy vs unhealthy proxies
- Per-node proxy status
- Last update timestamps

Controls:
- `q` - Quit
- `r` - Refresh manually
- Auto-refreshes every 2 seconds

## Security Considerations

1. **Firewall Rules**: Ensure proper firewall configuration
   - Agent API port (8080) should only be accessible from coordinator
   - Proxy ports (10000-20000) should be accessible as needed
   - Metrics ports should be restricted to monitoring systems

2. **Authentication**: Consider adding authentication for production:
   - API key authentication for agent-coordinator communication
   - Basic auth for proxy access

3. **TLS**: Use TLS for agent-coordinator communication in production

## Troubleshooting

### Agent not discovering IPv6 addresses

Check IPv6 connectivity:
```bash
ip -6 addr show
ping6 google.com
```

### Tinyproxy instances failing to start

Check tinyproxy installation:
```bash
which tinyproxy
tinyproxy -v
```

Check port availability:
```bash
netstat -tuln | grep LISTEN
```

### Coordinator not receiving updates

Check network connectivity between agent and coordinator:
```bash
curl http://coordinator-ip:8081/health
```

## License

MIT License

## Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.