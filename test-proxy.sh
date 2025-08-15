#!/bin/bash

# Test script for proxy functionality

if [ $# -lt 2 ]; then
    echo "Usage: $0 <ipv6_address> <port>"
    echo "Example: $0 2604:a880:cad:d0::ead8:700d 10002"
    exit 1
fi

IPV6_ADDR="$1"
PORT="$2"
PROXY="http://[$IPV6_ADDR]:$PORT"

echo "Testing proxy at $PROXY"

# Test 1: Basic connectivity
echo "1. Testing basic connectivity..."
if nc -z -6 "$IPV6_ADDR" "$PORT" 2>/dev/null; then
    echo "✓ TCP connection successful"
else
    echo "✗ TCP connection failed"
    exit 1
fi

# Test 2: HTTP proxy test
echo "2. Testing HTTP proxy functionality..."
if curl -x "$PROXY" --connect-timeout 10 -s http://httpbin.org/ip > /tmp/proxy-test.json; then
    echo "✓ HTTP proxy request successful"
    echo "Response:"
    cat /tmp/proxy-test.json | jq . 2>/dev/null || cat /tmp/proxy-test.json
    rm -f /tmp/proxy-test.json
else
    echo "✗ HTTP proxy request failed"
fi

# Test 3: HTTPS proxy test  
echo "3. Testing HTTPS proxy functionality..."
if curl -x "$PROXY" --connect-timeout 10 -s https://httpbin.org/ip > /tmp/proxy-test-https.json; then
    echo "✓ HTTPS proxy request successful"
    echo "Response:"
    cat /tmp/proxy-test-https.json | jq . 2>/dev/null || cat /tmp/proxy-test-https.json
    rm -f /tmp/proxy-test-https.json
else
    echo "✗ HTTPS proxy request failed"
fi

echo "Test completed."