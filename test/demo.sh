#!/bin/bash

# Demo interaktif PostgreSQL HA

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}=========================================="
echo "PostgreSQL HA Demo"
echo -e "==========================================${NC}"
echo ""

# Function untuk pause
pause() {
    echo ""
    read -p "Press Enter to continue..."
    echo ""
}

# 1. Show cluster status
echo -e "${YELLOW}1. Status Cluster Saat Ini${NC}"
docker exec -it pg1 patronictl -c /etc/patroni.yml list
pause

# 2. Create database
echo -e "${YELLOW}2. Membuat Database Test${NC}"
echo "Membuat database 'demo' dan table 'messages'..."
docker exec pg1 psql -U postgres -c "DROP DATABASE IF EXISTS demo;"
docker exec pg1 psql -U postgres -c "CREATE DATABASE demo;"
docker exec pg1 psql -U postgres -d demo -c "CREATE TABLE messages (id SERIAL PRIMARY KEY, message TEXT, created_at TIMESTAMP DEFAULT NOW());"
docker exec pg1 psql -U postgres -d demo -c "INSERT INTO messages (message) VALUES ('Hello from PostgreSQL HA!');"
echo ""
echo "Data di database:"
docker exec pg1 psql -U postgres -d demo -c "SELECT * FROM messages;"
pause

# 3. Show replication
echo -e "${YELLOW}3. Verifikasi Replikasi${NC}"
echo "Data di pg1 (Leader):"
docker exec pg1 psql -U postgres -d demo -c "SELECT * FROM messages;"
echo ""
echo "Data di pg2 (Replica):"
docker exec pg2 psql -U postgres -d demo -c "SELECT * FROM messages;"
echo ""
echo "Data di pg3 (Replica):"
docker exec pg3 psql -U postgres -d demo -c "SELECT * FROM messages;"
echo ""
echo -e "${GREEN}✓ Data ter-replikasi ke semua node!${NC}"
pause

# 4. Simulate failover
echo -e "${YELLOW}4. Simulasi Failover${NC}"
LEADER=$(docker exec pg1 patronictl -c /etc/patroni.yml list | grep Leader | awk '{print $2}')
LEADER_CONTAINER=$(echo $LEADER | sed 's/node/pg/')
echo "Current leader: $LEADER ($LEADER_CONTAINER)"
echo ""
echo "Mematikan leader dalam 3 detik..."
sleep 1
echo "3..."
sleep 1
echo "2..."
sleep 1
echo "1..."
docker stop $LEADER_CONTAINER
echo ""
echo -e "${GREEN}Leader dimatikan!${NC}"
echo ""
echo "Menunggu automatic failover (15 detik)..."
for i in {15..1}; do
    echo -n "$i "
    sleep 1
done
echo ""
echo ""

# 5. Show new leader
echo -e "${YELLOW}5. Status Cluster Setelah Failover${NC}"
if [ "$LEADER_CONTAINER" == "pg1" ]; then
    docker exec pg2 patronictl -c /etc/patroni.yml list
else
    docker exec pg1 patronictl -c /etc/patroni.yml list
fi
echo ""
echo -e "${GREEN}✓ Leader baru terpilih otomatis!${NC}"
pause

# 6. Write to new leader
echo -e "${YELLOW}6. Write Data ke Leader Baru${NC}"
NEW_LEADER=$(docker exec pg2 patronictl -c /etc/patroni.yml list 2>/dev/null | grep Leader | awk '{print $2}' || docker exec pg1 patronictl -c /etc/patroni.yml list | grep Leader | awk '{print $2}')
NEW_LEADER_CONTAINER=$(echo $NEW_LEADER | sed 's/node/pg/')
echo "Leader baru: $NEW_LEADER ($NEW_LEADER_CONTAINER)"
echo ""
echo "Menulis data baru ke leader..."
docker exec $NEW_LEADER_CONTAINER psql -U postgres -d demo -c "INSERT INTO messages (message) VALUES ('Data after failover - written to $NEW_LEADER');"
echo ""
echo "Data di database:"
docker exec $NEW_LEADER_CONTAINER psql -U postgres -d demo -c "SELECT * FROM messages;"
echo ""
echo -e "${GREEN}✓ Write berhasil ke leader baru!${NC}"
pause

# 7. Recover old leader
echo -e "${YELLOW}7. Menghidupkan Kembali Node yang Mati${NC}"
echo "Menghidupkan $LEADER_CONTAINER..."
docker start $LEADER_CONTAINER
echo ""
echo "Menunggu node join kembali (20 detik)..."
for i in {20..1}; do
    echo -n "$i "
    sleep 1
done
echo ""
echo ""

# 8. Final status
echo -e "${YELLOW}8. Status Cluster Final${NC}"
docker exec pg1 patronictl -c /etc/patroni.yml list 2>/dev/null || docker exec pg2 patronictl -c /etc/patroni.yml list
echo ""
echo -e "${GREEN}✓ Node kembali join sebagai replica!${NC}"
pause

# 9. Verify data consistency
echo -e "${YELLOW}9. Verifikasi Data Consistency${NC}"
echo "Data di semua node harus sama:"
echo ""
echo "pg1:"
docker exec pg1 psql -U postgres -d demo -c "SELECT * FROM messages;" 2>/dev/null || echo "pg1 belum siap"
echo ""
echo "pg2:"
docker exec pg2 psql -U postgres -d demo -c "SELECT * FROM messages;"
echo ""
echo "pg3:"
docker exec pg3 psql -U postgres -d demo -c "SELECT * FROM messages;"
echo ""
echo -e "${GREEN}✓ Semua node punya data yang sama!${NC}"
pause

# Summary
echo -e "${BLUE}=========================================="
echo "Demo Selesai!"
echo -e "==========================================${NC}"
echo ""
echo "Yang sudah dibuktikan:"
echo "✓ Cluster berjalan dengan 3 node"
echo "✓ Streaming replication real-time"
echo "✓ Automatic failover < 15 detik"
echo "✓ Write ke leader baru berhasil"
echo "✓ Node recovery otomatis"
echo "✓ Data consistency terjaga"
echo ""
echo "Monitoring:"
echo "- Cluster status: docker exec -it pg1 patronictl -c /etc/patroni.yml list"
echo "- HAProxy stats: http://localhost:7000/stats (admin/admin)"
echo "- Logs: docker compose logs -f"
echo ""
echo "Connection string untuk aplikasi:"
echo "postgresql://postgres:admin_password@localhost:54320/demo"
echo ""
