#!/bin/bash

# Test script to debug coordinator API responses

COORDINATOR_URL="${1:-http://localhost:8081}"

echo "=== Testing Coordinator API ==="
echo "Coordinator URL: $COORDINATOR_URL"
echo

# Test /api/nodes endpoint
echo "1. Testing /api/nodes endpoint:"
echo "   URL: $COORDINATOR_URL/api/nodes"
echo "   Response:"
curl -s -w "\n   HTTP Status: %{http_code}\n" "$COORDINATOR_URL/api/nodes" | head -20
echo

# Check if response starts with valid JSON
echo "2. Checking response format:"
RESPONSE=$(curl -s "$COORDINATOR_URL/api/nodes")
FIRST_CHAR=$(echo "$RESPONSE" | cut -c1)
echo "   First character of response: '$FIRST_CHAR'"

if [[ "$FIRST_CHAR" == "[" ]] || [[ "$FIRST_CHAR" == "{" ]]; then
    echo "   ✓ Response appears to be JSON"
else
    echo "   ✗ Response does NOT appear to be JSON"
    echo "   First 100 chars of response:"
    echo "$RESPONSE" | head -c 100
    echo
fi
echo

# Test /api/stats endpoint
echo "3. Testing /api/stats endpoint:"
echo "   URL: $COORDINATOR_URL/api/stats"
echo "   Response:"
curl -s -w "\n   HTTP Status: %{http_code}\n" "$COORDINATOR_URL/api/stats" | head -20
echo

# Check if coordinator is running
echo "4. Testing health endpoint:"
echo "   URL: $COORDINATOR_URL/health"
curl -s -w "\n   HTTP Status: %{http_code}\n" "$COORDINATOR_URL/health"
echo

# Check if it's actually the coordinator
echo "5. Checking if coordinator is running on this port:"
if curl -s "$COORDINATOR_URL/health" | grep -q "healthy"; then
    echo "   ✓ Coordinator appears to be running"
else
    echo "   ✗ Coordinator may not be running or responding correctly"
    echo "   Try starting the coordinator with: ./bin/coordinator"
fi