#!/bin/bash

# Quick test untuk cek status cluster

echo "=== Status Cluster ==="
docker exec -it pg1 patronictl -c /etc/patroni.yml list
echo ""

echo "=== Test Write & Read ==="
docker exec pg1 psql -U postgres -c "SELECT now() as current_time, version();"
echo ""

echo "=== HAProxy Stats ==="
echo "HAProxy Stats: http://localhost:7000/stats"
echo "Username: admin"
echo "Password: admin"
echo ""
