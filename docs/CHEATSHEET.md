# PostgreSQL HA Cheat Sheet

Quick reference untuk command yang sering digunakan.

## Cluster Management

```bash
# Start cluster
docker compose up -d

# Stop cluster
docker compose down

# Stop dan hapus data
docker compose down -v

# Restart cluster
docker compose restart

# Rebuild image
docker compose build
docker compose up -d

# View logs
docker compose logs -f
docker logs -f pg1
```

## Patroni Commands

```bash
# List cluster members
docker exec -it pg1 patronictl -c /etc/patroni.yml list

# Show cluster history
docker exec -it pg1 patronictl -c /etc/patroni.yml history

# Show cluster topology
docker exec -it pg1 patronictl -c /etc/patroni.yml topology

# Reload configuration
docker exec -it pg1 patronictl -c /etc/patroni.yml reload

# Restart node
docker exec -it pg1 patronictl -c /etc/patroni.yml restart node1

# Reinitialize node (WARNING: data loss!)
docker exec -it pg1 patronictl -c /etc/patroni.yml reinit node1
```

## Failover Commands

```bash
# Manual failover (interactive)
docker exec -it pg1 patronictl -c /etc/patroni.yml failover

# Failover to specific node
docker exec -it pg1 patronictl -c /etc/patroni.yml failover --candidate node2

# Switchover (graceful, planned)
docker exec -it pg1 patronictl -c /etc/patroni.yml switchover --candidate node2

# Switchover with scheduled time
docker exec -it pg1 patronictl -c /etc/patroni.yml switchover --scheduled "2025-11-15T20:00:00"
```

## PostgreSQL Connection

```bash
# Via HAProxy (recommended for apps)
PGPASSWORD=admin_password psql -h localhost -p 54320 -U postgres

# Direct to nodes
PGPASSWORD=admin_password psql -h localhost -p 5431 -U postgres  # pg1
PGPASSWORD=admin_password psql -h localhost -p 5432 -U postgres  # pg2
PGPASSWORD=admin_password psql -h localhost -p 5433 -U postgres  # pg3

# Connection string
postgresql://postgres:admin_password@localhost:54320/postgres
```

## PostgreSQL Queries

```sql
-- Check replication status
SELECT * FROM pg_stat_replication;

-- Check if node is primary or standby
SELECT pg_is_in_recovery();

-- Show current timeline
SELECT timeline_id FROM pg_control_checkpoint();

-- Show replication lag
SELECT 
    client_addr,
    state,
    sent_lsn,
    write_lsn,
    flush_lsn,
    replay_lsn,
    sync_state
FROM pg_stat_replication;

-- Show database size
SELECT 
    datname,
    pg_size_pretty(pg_database_size(datname)) as size
FROM pg_database;
```

## Monitoring

```bash
# Patroni REST API
curl http://localhost:8008/health      # pg1 health
curl http://localhost:8008/leader      # who is leader
curl http://localhost:8008/replica     # replica status
curl http://localhost:8008/patroni     # full status

# HAProxy stats
# Browser: http://localhost:7000/stats
# Username: admin, Password: admin

# etcd status
docker exec etcd etcdctl endpoint health
docker exec etcd etcdctl member list

# Check cluster data in etcd
docker exec etcd etcdctl get --prefix "/db/"
```

## Testing

```bash
# Quick status check
./quick-test.sh

# Full HA test (includes failover)
./test-ha.sh

# Interactive demo
./demo.sh
```

## Troubleshooting

```bash
# Check if containers are running
docker ps

# Check container logs
docker logs pg1 --tail 100
docker logs pg1 -f

# Check network connectivity
docker exec pg1 ping pg2
docker exec pg1 curl http://etcd:2379/version

# Check PostgreSQL is running
docker exec pg1 pg_isready -U postgres

# Check Patroni config
docker exec pg1 cat /etc/patroni.yml

# Enter container shell
docker exec -it pg1 bash

# Check disk space
docker exec pg1 df -h

# Check PostgreSQL processes
docker exec pg1 ps aux | grep postgres
```

## Maintenance Mode

```bash
# Pause Patroni (prevent automatic failover)
docker exec -it pg1 patronictl -c /etc/patroni.yml pause

# Resume Patroni
docker exec -it pg1 patronictl -c /etc/patroni.yml resume

# Enable maintenance mode for specific node
docker exec -it pg1 patronictl -c /etc/patroni.yml edit-config
# Set: "pause": true
```

## Backup & Restore

```bash
# Backup database
docker exec pg1 pg_dump -U postgres -d mydb > backup.sql

# Backup all databases
docker exec pg1 pg_dumpall -U postgres > backup_all.sql

# Restore database
cat backup.sql | docker exec -i pg1 psql -U postgres -d mydb

# Backup with compression
docker exec pg1 pg_dump -U postgres -d mydb | gzip > backup.sql.gz

# Restore from compressed backup
gunzip -c backup.sql.gz | docker exec -i pg1 psql -U postgres -d mydb
```

## Performance Tuning

```sql
-- Show slow queries
SELECT 
    pid,
    now() - query_start as duration,
    query
FROM pg_stat_activity
WHERE state = 'active'
ORDER BY duration DESC;

-- Show table sizes
SELECT 
    schemaname,
    tablename,
    pg_size_pretty(pg_total_relation_size(schemaname||'.'||tablename)) as size
FROM pg_tables
ORDER BY pg_total_relation_size(schemaname||'.'||tablename) DESC
LIMIT 10;

-- Show index usage
SELECT 
    schemaname,
    tablename,
    indexname,
    idx_scan,
    idx_tup_read,
    idx_tup_fetch
FROM pg_stat_user_indexes
ORDER BY idx_scan DESC;

-- Show cache hit ratio
SELECT 
    sum(heap_blks_read) as heap_read,
    sum(heap_blks_hit) as heap_hit,
    sum(heap_blks_hit) / (sum(heap_blks_hit) + sum(heap_blks_read)) as ratio
FROM pg_statio_user_tables;
```

## Security

```bash
# Change postgres password
docker exec pg1 psql -U postgres -c "ALTER USER postgres PASSWORD 'new_password';"

# Create new user
docker exec pg1 psql -U postgres -c "CREATE USER myapp WITH PASSWORD 'mypassword';"

# Grant privileges
docker exec pg1 psql -U postgres -c "GRANT ALL PRIVILEGES ON DATABASE mydb TO myapp;"

# Show users
docker exec pg1 psql -U postgres -c "\du"

# Show databases
docker exec pg1 psql -U postgres -c "\l"
```

## Quick Reference

| Task | Command |
|------|---------|
| Start cluster | `docker compose up -d` |
| Stop cluster | `docker compose down` |
| View status | `docker exec -it pg1 patronictl -c /etc/patroni.yml list` |
| Connect to DB | `PGPASSWORD=admin_password psql -h localhost -p 54320 -U postgres` |
| View logs | `docker compose logs -f` |
| Manual failover | `docker exec -it pg1 patronictl -c /etc/patroni.yml failover` |
| HAProxy stats | http://localhost:7000/stats |
| Run tests | `./test-ha.sh` |

## Connection Strings

```bash
# For applications (via HAProxy)
postgresql://postgres:admin_password@localhost:54320/postgres

# Direct connections
postgresql://postgres:admin_password@localhost:5431/postgres  # pg1
postgresql://postgres:admin_password@localhost:5432/postgres  # pg2
postgresql://postgres:admin_password@localhost:5433/postgres  # pg3
```

## Environment Variables

```bash
# Patroni
PATRONI_SCOPE=pgcluster
PATRONI_NAME=node1

# PostgreSQL
POSTGRES_PASSWORD=admin_password
POSTGRES_USER=postgres
```
