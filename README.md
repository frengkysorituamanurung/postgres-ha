# PostgreSQL High Availability dengan Patroni, etcd, dan HAProxy

Setup PostgreSQL HA cluster dengan 3 node menggunakan Patroni untuk automatic failover, etcd sebagai DCS (Distributed Configuration Store), dan HAProxy sebagai load balancer.

## Arsitektur

```
┌─────────────┐
│   HAProxy   │ :54320 (aplikasi connect ke sini)
│ Load Balancer│
└──────┬──────┘
       │
       ├─────────┬─────────┐
       │         │         │
   ┌───▼───┐ ┌──▼────┐ ┌──▼────┐
   │  pg1  │ │  pg2  │ │  pg3  │
   │Leader │ │Replica│ │Replica│
   │:5431  │ │:5432  │ │:5433  │
   └───┬───┘ └───┬───┘ └───┬───┘
       │         │         │
       └─────────┴─────────┘
              │
         ┌────▼────┐
         │  etcd   │ :2379
         │   DCS   │
         └─────────┘
```

## Komponen

- **PostgreSQL 16**: Database server
- **Patroni 3.x**: HA orchestration tool
- **etcd 3.5**: Distributed key-value store untuk cluster state
- **HAProxy 2.9**: Load balancer yang route traffic ke leader

## Quick Start

### 1. Start Cluster

```bash
docker compose up -d
```

### 2. Cek Status Cluster

```bash
./quick-test.sh
```

Atau manual:

```bash
docker exec -it pg1 patronictl -c /etc/patroni.yml list
```

Output:
```
+ Cluster: pgcluster (7572980080390463510) +-------------+-----+------------+-----+
| Member | Host | Role    | State     | TL | Receive LSN | Lag | Replay LSN | Lag |
+--------+------+---------+-----------+----+-------------+-----+------------+-----+
| node1  | pg1  | Leader  | running   |  1 |             |     |            |     |
| node2  | pg2  | Replica | streaming |  1 |   0/404D640 |   0 |  0/404D640 |   0 |
| node3  | pg3  | Replica | streaming |  1 |   0/404D640 |   0 |  0/404D640 |   0 |
+--------+------+---------+-----------+----+-------------+-----+------------+-----+
```

### 3. Koneksi ke Database

**Via HAProxy (Recommended untuk aplikasi):**
```bash
psql -h localhost -p 54320 -U postgres
```

**Direct ke node:**
```bash
# Leader (pg1)
psql -h localhost -p 5431 -U postgres

# Replica (pg2)
psql -h localhost -p 5432 -U postgres

# Replica (pg3)
psql -h localhost -p 5433 -U postgres
```

**Credentials:**
- Username: `postgres`
- Password: `admin_password`

## Testing High Availability

Jalankan full HA test:

```bash
./test-ha.sh
```

Test ini akan:
1. ✓ Cek status cluster
2. ✓ Buat database dan table test
3. ✓ Verifikasi replikasi ke semua node
4. ✓ Simulasi failover (matikan leader)
5. ✓ Verifikasi leader baru terpilih otomatis
6. ✓ Test write ke leader baru
7. ✓ Hidupkan kembali node yang mati
8. ✓ Verifikasi data consistency

## Monitoring

### HAProxy Stats Page

Buka browser: http://localhost:7000/stats

- Username: `admin`
- Password: `admin`

Di sini bisa lihat:
- Node mana yang aktif sebagai leader
- Health check status
- Connection statistics

### Patroni REST API

Cek status via REST API:

```bash
# Node 1
curl http://localhost:8008/patroni

# Node 2  
curl http://localhost:8009/patroni

# Node 3
curl http://localhost:8010/patroni
```

## Manual Failover

Jika ingin manual failover (misalnya untuk maintenance):

```bash
# Failover ke node2
docker exec -it pg1 patronictl -c /etc/patroni.yml failover --candidate node2

# Atau switchover (lebih graceful)
docker exec -it pg1 patronictl -c /etc/patroni.yml switchover --candidate node2
```

## Maintenance

### Stop Cluster

```bash
docker compose down
```

### Stop dan Hapus Data

```bash
docker compose down -v
```

### Restart Node Tertentu

```bash
docker compose restart pg1
```

### Rebuild Image

```bash
docker compose build
docker compose up -d
```

## Troubleshooting

### Cek Log

```bash
# Log semua container
docker compose logs -f

# Log node tertentu
docker logs -f pg1
docker logs -f pg2
docker logs -f pg3
docker logs -f etcd
docker logs -f haproxy
```

### Cluster Tidak Initialize

```bash
# Hapus semua data dan start ulang
docker compose down -v
docker compose up -d
```

### Node Tidak Join Cluster

```bash
# Cek koneksi ke etcd
docker exec pg1 curl http://etcd:2379/version

# Cek Patroni config
docker exec pg1 cat /etc/patroni.yml
```

## Configuration Files

- `docker-compose.yml`: Container orchestration
- `Dockerfile.patroni`: Custom Patroni image dengan etcd3 support
- `patroni/node1.yml`: Patroni config untuk node1 (dengan bootstrap)
- `patroni/node2.yml`: Patroni config untuk node2
- `patroni/node3.yml`: Patroni config untuk node3
- `haproxy/haproxy.cfg`: HAProxy load balancer config

## Fitur HA

✅ **Automatic Failover**: Jika leader mati, replica otomatis jadi leader (< 30 detik)
✅ **Streaming Replication**: Data real-time sync ke semua replica
✅ **Split-brain Protection**: etcd mencegah multiple leader
✅ **Health Checks**: Patroni monitor PostgreSQL health
✅ **Load Balancing**: HAProxy route traffic ke leader aktif
✅ **Zero Downtime**: Aplikasi tetap bisa connect via HAProxy saat failover

## Production Considerations

Untuk production, pertimbangkan:

1. **Persistent Storage**: Gunakan volume untuk data persistence
2. **Backup**: Setup automated backup (pg_basebackup, WAL archiving)
3. **Monitoring**: Integrate dengan Prometheus/Grafana
4. **Security**: 
   - Ganti password default
   - Setup SSL/TLS
   - Network isolation
5. **Resource Limits**: Set memory dan CPU limits di docker-compose
6. **Multi-AZ**: Deploy node di availability zone berbeda

## 📖 Detailed Documentation

Untuk pemahaman mendalam tentang mekanisme HA:

- **[HA Mechanism](docs/HA-MECHANISM.md)** - Penjelasan lengkap arsitektur dan mekanisme HA
- **[Failover Flow](docs/FAILOVER-FLOW.md)** - Visualisasi detail proses failover step-by-step
- **[Patroni Internals](docs/PATRONI-INTERNALS.md)** - Deep dive ke internal Patroni
- **[Testing Guide](docs/TESTING.md)** - Hasil testing dan troubleshooting
- **[Cheat Sheet](docs/CHEATSHEET.md)** - Quick reference commands

## License

MIT
