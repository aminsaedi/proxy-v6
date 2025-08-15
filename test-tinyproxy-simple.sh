#!/bin/bash

# Simple test to verify tinyproxy can bind to IPv6 addresses

echo "=== Simple Tinyproxy IPv6 Test ==="

# Check for IPv6 addresses
echo "Available IPv6 addresses:"
ip -6 addr show | grep "inet6" | grep -v "fe80" | grep -v "::1" | awk '{print $2}' | cut -d'/' -f1

echo ""
echo "Enter an IPv6 address to test (or press Enter to auto-select):"
read IPV6_ADDR

if [ -z "$IPV6_ADDR" ]; then
    IPV6_ADDR=$(ip -6 addr show | grep "inet6" | grep -v "fe80" | grep -v "::1" | head -1 | awk '{print $2}' | cut -d'/' -f1)
    echo "Auto-selected: $IPV6_ADDR"
fi

if [ -z "$IPV6_ADDR" ]; then
    echo "No IPv6 address available"
    exit 1
fi

PORT=18888
CONFIG="/tmp/simple-test.conf"

# Create minimal config
cat > $CONFIG << EOF
Port $PORT
Listen $IPV6_ADDR
LogLevel Info
LogFile /tmp/simple-test.log
PidFile /tmp/simple-test.pid
Allow ::1
Allow 127.0.0.1
EOF

echo ""
echo "Config created:"
cat $CONFIG

echo ""
echo "Starting tinyproxy..."
tinyproxy -c $CONFIG

sleep 2

echo ""
echo "Checking if tinyproxy is running..."
if pgrep -f "tinyproxy.*simple-test" > /dev/null; then
    echo "✓ Tinyproxy process is running"
    
    echo ""
    echo "Checking if port is listening..."
    if netstat -tln | grep -q ":$PORT "; then
        echo "✓ Port $PORT is listening"
    else
        echo "✗ Port $PORT is NOT listening"
    fi
    
    echo ""
    echo "Testing connection..."
    if timeout 2 nc -6 $IPV6_ADDR $PORT < /dev/null; then
        echo "✓ Can connect to proxy"
    else
        echo "✗ Cannot connect to proxy"
    fi
    
    echo ""
    echo "Stopping tinyproxy..."
    pkill -f "tinyproxy.*simple-test"
else
    echo "✗ Tinyproxy failed to start"
    
    echo ""
    echo "Log file contents:"
    cat /tmp/simple-test.log 2>/dev/null || echo "No log file"
fi

echo ""
echo "Cleaning up..."
rm -f $CONFIG /tmp/simple-test.log /tmp/simple-test.pid

echo ""
echo "Test complete!"