#!/bin/bash

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}║   PostgreSQL HA Web UI                                      ║${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Check if cluster is running
echo -e "${YELLOW}[1/3] Checking cluster status...${NC}"
if ! docker ps | grep -q pg1; then
    echo -e "${YELLOW}Cluster is not running. Starting...${NC}"
    docker compose up -d
    sleep 15
fi

docker exec -it pg1 patronictl -c /etc/patroni.yml list 2>/dev/null || docker exec -it pg2 patronictl -c /etc/patroni.yml list
echo ""

# Build web UI
echo -e "${YELLOW}[2/3] Building Web UI...${NC}"
cd web
docker build -t postgres-ha-web . > /dev/null 2>&1
echo -e "${GREEN}✓ Build successful!${NC}"
echo ""

# Run web UI
echo -e "${YELLOW}[3/3] Starting Web UI...${NC}"
echo -e "${GREEN}✓ Web UI starting...${NC}"
echo ""
echo -e "${BLUE}╔══════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}║   🌐 Open your browser:                                     ║${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}║   http://localhost:8080                                     ║${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}║   Press Ctrl+C to stop                                      ║${NC}"
echo -e "${BLUE}║                                                              ║${NC}"
echo -e "${BLUE}╚══════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Run with docker socket mounted for docker commands
docker run --rm \
    --network host \
    -v /var/run/docker.sock:/var/run/docker.sock \
    postgres-ha-web
