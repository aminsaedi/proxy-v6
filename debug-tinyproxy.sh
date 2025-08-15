#!/bin/bash

# Debug script for tinyproxy issues

set -e

echo "=== Tinyproxy Debug Script ==="
echo

# Check if tinyproxy is installed
echo "1. Checking tinyproxy installation..."
if command -v tinyproxy &> /dev/null; then
    echo "✓ Tinyproxy is installed"
    tinyproxy -v 2>&1 | head -1 || echo "Version info not available"
else
    echo "✗ Tinyproxy is NOT installed!"
    echo "Install with: apt-get install tinyproxy"
    exit 1
fi
echo

# Check IPv6 addresses
echo "2. Checking IPv6 addresses..."
ip -6 addr show | grep "inet6" | grep -v "fe80" | grep -v "::1" || echo "No public IPv6 addresses found"
echo

# Test creating a minimal config
echo "3. Testing minimal tinyproxy config..."
TEST_CONFIG="/tmp/test-tinyproxy.conf"
TEST_PORT=19999

# Get first public IPv6 address
IPV6_ADDR=$(ip -6 addr show | grep "inet6" | grep -v "fe80" | grep -v "::1" | head -1 | awk '{print $2}' | cut -d'/' -f1)

if [ -z "$IPV6_ADDR" ]; then
    echo "✗ No public IPv6 address found"
    exit 1
fi

echo "Using IPv6 address: $IPV6_ADDR"

cat > $TEST_CONFIG << EOF
Port $TEST_PORT
Listen $IPV6_ADDR
MaxClients 10
StartServers 2
MinSpareServers 1
MaxSpareServers 2
MaxRequestsPerChild 100
Allow ::1
Allow 127.0.0.1
Allow $IPV6_ADDR
LogLevel Info
LogFile /tmp/test-tinyproxy.log
PidFile /tmp/test-tinyproxy.pid
EOF

echo "Config file created at: $TEST_CONFIG"
cat $TEST_CONFIG
echo

# Try to start tinyproxy in debug mode
echo "4. Starting tinyproxy in debug mode..."
echo "Running: tinyproxy -d -c $TEST_CONFIG"
timeout 5 tinyproxy -d -c $TEST_CONFIG 2>&1 || true
echo

# Check if it created a log
echo "5. Checking log file..."
if [ -f /tmp/test-tinyproxy.log ]; then
    echo "Log file contents:"
    cat /tmp/test-tinyproxy.log
else
    echo "No log file created"
fi
echo

# Check for running tinyproxy processes
echo "6. Checking for tinyproxy processes..."
ps aux | grep tinyproxy | grep -v grep || echo "No tinyproxy processes running"
echo

# Check if port is listening
echo "7. Checking if port $TEST_PORT is listening..."
netstat -tlnp 2>/dev/null | grep $TEST_PORT || ss -tlnp | grep $TEST_PORT || echo "Port $TEST_PORT is not listening"
echo

# Clean up
echo "8. Cleaning up..."
pkill -f "tinyproxy.*test-tinyproxy" 2>/dev/null || true
rm -f $TEST_CONFIG /tmp/test-tinyproxy.log /tmp/test-tinyproxy.pid
echo "✓ Cleanup complete"

echo
echo "=== Debug Complete ==="
echo
echo "If tinyproxy failed to start, common issues include:"
echo "- IPv6 address not properly configured on the interface"
echo "- Port already in use"
echo "- Permission issues (try running as root)"
echo "- Missing dependencies or incorrect tinyproxy version"