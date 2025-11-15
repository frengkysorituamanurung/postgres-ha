# Mekanisme High Availability PostgreSQL dengan Patroni

## 📚 Table of Contents

1. [Arsitektur Overview](#arsitektur-overview)
2. [Komponen dan Perannya](#komponen-dan-perannya)
3. [Mekanisme Failover](#mekanisme-failover)
4. [Streaming Replication](#streaming-replication)
5. [Leader Election](#leader-election)
6. [Health Checking](#health-checking)
7. [Split-Brain Prevention](#split-brain-prevention)
8. [Recovery Process](#recovery-process)
9. [Timeline Management](#timeline-management)
10. [Skenario Real-World](#skenario-real-world)

---

## 1. Arsitektur Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                         APLIKASI                                │
│                    (Connection Pool)                            │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         │ Port 54320
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                       HAProxy                                   │
│                   (Load Balancer)                               │
│  - Health check ke Patroni REST API                            │
│  - Route traffic hanya ke Leader                               │
│  - Automatic failover detection                                │
└──────┬──────────────────┬──────────────────┬────────────────────┘
       │                  │                  │
       │ Health Check     │ Health Check     │ Health Check
       │ :8008/primary    │ :8008/primary    │ :8008/primary
       ▼                  ▼                  ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│   Patroni    │   │   Patroni    │   │   Patroni    │
│    node1     │   │    node2     │   │    node3     │
│  (Leader)    │   │  (Replica)   │   │  (Replica)   │
│              │   │              │   │              │
│ REST API     │   │ REST API     │   │ REST API     │
│ :8008        │   │ :8008        │   │ :8008        │
└──────┬───────┘   └──────┬───────┘   └──────┬───────┘
       │                  │                  │
       │ Manage           │ Manage           │ Manage
       ▼                  ▼                  ▼
┌──────────────┐   ┌──────────────┐   ┌──────────────┐
│ PostgreSQL   │   │ PostgreSQL   │   │ PostgreSQL   │
│    pg1       │   │    pg2       │   │    pg3       │
│  :5432       │   │  :5432       │   │  :5432       │
│              │   │              │   │              │
│ Primary      │──▶│ Standby      │   │ Standby      │
│ (Read/Write) │   │ (Read Only)  │   │ (Read Only)  │
└──────┬───────┘   └──────┬───────┘   └──────┬───────┘
       │                  │                  │
       │ WAL Streaming    │                  │
       └──────────────────┴──────────────────┘
                         │
                         │ Cluster State
                         │ Leader Lock
                         │ Configuration
                         ▼
                  ┌──────────────┐
                  │     etcd     │
                  │ (DCS Store)  │
                  │   :2379      │
                  └──────────────┘
```

---

## 2. Komponen dan Perannya

### 2.1 PostgreSQL (Database Engine)

**Peran:**
- Menyimpan dan mengelola data
- Melakukan replikasi WAL (Write-Ahead Log)
- Menjalankan query dari aplikasi

**Mode Operasi:**
- **Primary (Leader)**: Menerima write & read, generate WAL
- **Standby (Replica)**: Hanya read, consume WAL dari primary

**File Penting:**
```
/home/postgres/data/
├── postgresql.conf      # Config PostgreSQL
├── pg_hba.conf         # Authentication rules
├── postmaster.pid      # Process ID file
├── pg_wal/             # WAL files
└── recovery.signal     # Standby mode indicator
```

### 2.2 Patroni (HA Orchestrator)

**Peran:**
- Monitor kesehatan PostgreSQL
- Manage lifecycle PostgreSQL (start, stop, restart)
- Koordinasi failover dan switchover
- Update cluster state ke etcd
- Expose REST API untuk health check

**Proses yang Berjalan:**
```python
# Patroni main loop (setiap 10 detik)
while True:
    1. Check PostgreSQL health
    2. Try to acquire/renew leader lock in etcd
    3. Update member info in etcd
    4. Check if need to follow new leader
    5. Respond to REST API requests
    sleep(loop_wait)  # Default: 10 seconds
```

**REST API Endpoints:**
```bash
GET /health         # Overall health status
GET /leader         # Returns 200 if this node is leader
GET /primary        # Alias for /leader (used by HAProxy)
GET /replica        # Returns 200 if this node is replica
GET /patroni        # Full cluster status
POST /restart       # Restart PostgreSQL
POST /reload        # Reload configuration
```

**Configuration Key Points:**
```yaml
scope: pgcluster              # Cluster name
namespace: /db/               # etcd namespace
name: node1                   # Node name

restapi:
  listen: 0.0.0.0:8008       # REST API port
  connect_address: pg1:8008   # Advertised address

etcd3:
  host: etcd:2379            # etcd connection

bootstrap:                    # Only on first node
  dcs:
    ttl: 30                  # Leader lock TTL
    loop_wait: 10            # Check interval
    retry_timeout: 30        # Retry timeout
    
postgresql:
  listen: 0.0.0.0:5432
  connect_address: pg1:5432
  data_dir: /home/postgres/data
  authentication:
    replication:
      username: replicator
      password: rep_password
    superuser:
      username: postgres
      password: admin_password
```

### 2.3 etcd (Distributed Configuration Store)

**Peran:**
- Menyimpan cluster state
- Distributed leader election
- Configuration management
- Prevent split-brain

**Data yang Disimpan:**
```
/db/pgcluster/
├── leader                    # Current leader info + lock
├── initialize                # Cluster initialization key
├── config                    # Cluster configuration
├── members/
│   ├── node1                # node1 status & info
│   ├── node2                # node2 status & info
│   └── node3                # node3 status & info
├── history                   # Failover history
└── failover                  # Failover trigger key
```

**Leader Lock Mechanism:**
```json
{
  "key": "/db/pgcluster/leader",
  "value": "node1",
  "ttl": 30,
  "lease_id": "7572983255259566102"
}
```

**Cara Kerja Lock:**
1. Leader acquire lock dengan TTL 30 detik
2. Leader renew lock setiap 10 detik (loop_wait)
3. Jika leader mati, lock expire setelah 30 detik
4. Replica detect lock expired, start election
5. Replica dengan priority tertinggi acquire lock

### 2.4 HAProxy (Load Balancer)

**Peran:**
- Route traffic ke leader yang aktif
- Health check semua node
- Automatic failover detection
- Connection pooling

**Configuration:**
```haproxy
backend pg_backend
    balance roundrobin
    option httpchk GET /primary        # Check if node is primary
    http-check expect status 200       # Expect 200 OK
    server pg1 pg1:5432 check port 8008
    server pg2 pg2:5432 check port 8008 backup
    server pg3 pg3:5432 check port 8008 backup
```

**Health Check Flow:**
```
HAProxy → GET http://pg1:8008/primary
         ← 200 OK (if leader)
         ← 503 Service Unavailable (if replica)

HAProxy → GET http://pg2:8008/primary
         ← 503 Service Unavailable (if replica)

HAProxy → GET http://pg3:8008/primary
         ← 503 Service Unavailable (if replica)

Result: HAProxy routes all traffic to pg1
```

---

## 3. Mekanisme Failover

### 3.1 Automatic Failover Flow

```
Timeline: Leader (pg1) Crashes

T=0s    pg1 crashes (hardware failure, OOM, etc)
        │
        ├─ PostgreSQL process dies
        ├─ Patroni on pg1 stops responding
        └─ Leader lock in etcd not renewed

T=10s   pg2 & pg3 detect leader is down
        │
        ├─ Patroni loop detects no leader lock
        ├─ Check if can become leader
        └─ Start leader election

T=15s   Leader Election
        │
        ├─ pg2 tries to acquire leader lock
        ├─ pg3 tries to acquire leader lock
        ├─ etcd grants lock to first requester (pg2)
        └─ pg2 becomes new leader

T=20s   pg2 promotes itself to primary
        │
        ├─ Remove recovery.signal file
        ├─ Restart PostgreSQL in primary mode
        ├─ Start accepting writes
        └─ Update cluster state in etcd

T=25s   pg3 detects new leader
        │
        ├─ Read leader info from etcd
        ├─ Reconfigure replication to follow pg2
        ├─ Restart PostgreSQL in standby mode
        └─ Start streaming from pg2

T=30s   HAProxy detects new leader
        │
        ├─ Health check to pg1: FAIL
        ├─ Health check to pg2: 200 OK (primary)
        ├─ Health check to pg3: 503 (replica)
        └─ Route all traffic to pg2

T=35s   Cluster fully operational
        │
        ├─ pg2 is new leader (Timeline 2)
        ├─ pg3 is replica streaming from pg2
        ├─ Applications connected via HAProxy
        └─ No data loss (synchronous replication)
```

### 3.2 Detailed Failover Steps

**Step 1: Failure Detection**
```python
# Patroni on pg2 & pg3
def check_leader():
    leader_info = etcd.get('/db/pgcluster/leader')
    if leader_info is None or leader_info.expired():
        # Leader is down!
        return None
    return leader_info

# Every 10 seconds
if check_leader() is None:
    start_election()
```

**Step 2: Leader Election**
```python
def start_election():
    # Check if eligible to be leader
    if not can_be_leader():
        return False
    
    # Try to acquire leader lock
    try:
        etcd.put('/db/pgcluster/leader', 
                 value=my_name,
                 ttl=30,
                 prevExist=False)  # Only if not exists
        return True
    except AlreadyExists:
        # Another node won the election
        return False
```

**Step 3: Promotion to Primary**
```python
def promote_to_primary():
    # 1. Remove recovery.signal (standby indicator)
    os.remove('/home/postgres/data/recovery.signal')
    
    # 2. Update postgresql.conf
    update_config({
        'hot_standby': 'off',
        'wal_level': 'replica'
    })
    
    # 3. Restart PostgreSQL
    pg_ctl('restart')
    
    # 4. Wait for PostgreSQL to accept connections
    wait_for_postgres()
    
    # 5. Update cluster state
    etcd.put('/db/pgcluster/members/node2', {
        'role': 'master',
        'state': 'running',
        'timeline': 2
    })
```

**Step 4: Replica Reconfiguration**
```python
def follow_new_leader(new_leader):
    # 1. Get new leader connection info
    leader_info = etcd.get(f'/db/pgcluster/members/{new_leader}')
    
    # 2. Update recovery.conf
    update_recovery_conf({
        'primary_conninfo': f"host={leader_info.host} port=5432 "
                           f"user=replicator password=rep_password"
    })
    
    # 3. Create recovery.signal
    touch('/home/postgres/data/recovery.signal')
    
    # 4. Restart PostgreSQL
    pg_ctl('restart')
    
    # 5. Wait for replication to start
    wait_for_streaming()
```

---

## 4. Streaming Replication

### 4.1 WAL (Write-Ahead Log) Mechanism

```
Primary (pg1):
┌─────────────────────────────────────────┐
│  1. Client: INSERT INTO users ...      │
│     ↓                                   │
│  2. Write to WAL buffer                 │
│     ↓                                   │
│  3. Flush WAL to disk                   │
│     ↓                                   │
│  4. Apply changes to data files         │
│     ↓                                   │
│  5. Send WAL to replicas                │
│     ↓                                   │
│  6. Wait for replica ACK (sync mode)    │
│     ↓                                   │
│  7. Return success to client            │
└─────────────────────────────────────────┘
         │
         │ WAL Stream (TCP)
         ▼
Replica (pg2, pg3):
┌─────────────────────────────────────────┐
│  1. Receive WAL from primary            │
│     ↓                                   │
│  2. Write WAL to local disk             │
│     ↓                                   │
│  3. Send ACK to primary                 │
│     ↓                                   │
│  4. Apply WAL to data files             │
│     ↓                                   │
│  5. Update replay position              │
└─────────────────────────────────────────┘
```

### 4.2 Replication Slots

**Purpose:** Prevent primary from deleting WAL files that replicas still need

```sql
-- On primary
SELECT * FROM pg_replication_slots;

 slot_name | slot_type | active | restart_lsn | confirmed_flush_lsn
-----------+-----------+--------+-------------+--------------------
 node2     | physical  | t      | 0/4000000   | 0/4000060
 node3     | physical  | t      | 0/4000000   | 0/4000060
```

**Configuration:**
```yaml
bootstrap:
  dcs:
    postgresql:
      use_slots: true           # Enable replication slots
      max_replication_slots: 10
```

### 4.3 Replication Lag Monitoring

```sql
-- On primary: Check replication status
SELECT 
    client_addr,
    state,
    sent_lsn,
    write_lsn,
    flush_lsn,
    replay_lsn,
    sync_state,
    pg_wal_lsn_diff(sent_lsn, replay_lsn) AS lag_bytes
FROM pg_stat_replication;

 client_addr | state     | sent_lsn  | replay_lsn | lag_bytes
-------------+-----------+-----------+------------+-----------
 172.28.0.3  | streaming | 0/4000060 | 0/4000060  | 0
 172.28.0.4  | streaming | 0/4000060 | 0/4000060  | 0
```

---

## 5. Leader Election

### 5.1 Election Algorithm

```
Eligibility Criteria:
1. Node is healthy (PostgreSQL running)
2. Node has latest data (highest LSN)
3. Node is not in maintenance mode
4. Node has no "nofailover" tag

Election Process:
1. All eligible nodes try to acquire leader lock
2. etcd grants lock to first requester (atomic operation)
3. Winner becomes leader
4. Losers become replicas and follow winner
```

### 5.2 Priority-Based Election

```yaml
# In patroni config
tags:
  nofailover: false      # Can become leader
  noloadbalance: false   # Can receive traffic
  clonefrom: true        # Can be cloned from
  nosync: false          # Can be synchronous replica
```

**Priority Calculation:**
```python
def calculate_priority(node):
    priority = 0
    
    # Higher LSN = higher priority
    priority += node.lsn / 1000000
    
    # Prefer nodes with lower lag
    priority -= node.lag_bytes / 1000000
    
    # Prefer nodes with nofailover=false
    if node.tags.get('nofailover'):
        priority -= 1000
    
    return priority

# Node with highest priority wins
winner = max(eligible_nodes, key=calculate_priority)
```

---

## 6. Health Checking

### 6.1 Patroni Health Checks

```python
# Patroni performs these checks every loop_wait (10s)

def check_postgresql_health():
    checks = {
        'process': check_process_running(),
        'connection': check_can_connect(),
        'replication': check_replication_status(),
        'lag': check_replication_lag(),
        'timeline': check_timeline_match()
    }
    
    return all(checks.values())

def check_process_running():
    # Check if postmaster process exists
    return os.path.exists('/home/postgres/data/postmaster.pid')

def check_can_connect():
    # Try to connect to PostgreSQL
    try:
        conn = psycopg2.connect(
            host='localhost',
            port=5432,
            user='postgres',
            password='admin_password',
            connect_timeout=3
        )
        conn.close()
        return True
    except:
        return False

def check_replication_status():
    # For replicas: check if streaming from primary
    if is_replica():
        result = query("SELECT status FROM pg_stat_wal_receiver")
        return result[0]['status'] == 'streaming'
    return True

def check_replication_lag():
    # Check if lag is within acceptable limits
    if is_replica():
        lag = query("SELECT pg_wal_lsn_diff(pg_last_wal_receive_lsn(), pg_last_wal_replay_lsn())")
        return lag < MAX_LAG_BYTES
    return True
```

### 6.2 HAProxy Health Checks

```
Every 2 seconds (default):

HAProxy → GET http://pg1:8008/primary
         ← HTTP 200 OK
         ← Body: {"state": "running", "role": "master"}

HAProxy → GET http://pg2:8008/primary
         ← HTTP 503 Service Unavailable
         ← Body: {"state": "running", "role": "replica"}

HAProxy marks pg1 as UP, pg2 as DOWN (for primary traffic)
```

---

## 7. Split-Brain Prevention

### 7.1 What is Split-Brain?

```
BAD SCENARIO (Without Protection):

Network Partition:
┌──────────┐         ┌──────────┐
│   pg1    │  ╳╳╳╳╳  │   pg2    │
│ (Leader) │         │ (Replica)│
└──────────┘         └──────────┘
     │                    │
     │ Can't communicate  │
     │                    │
     ▼                    ▼
Both think they are leader!
→ Data divergence
→ Data loss
→ Corruption
```

### 7.2 How Patroni Prevents Split-Brain

**1. Leader Lock with TTL**
```python
# Leader must renew lock every 10 seconds
# Lock expires after 30 seconds if not renewed

def maintain_leader_lock():
    while is_leader():
        try:
            etcd.refresh_lease(leader_lock, ttl=30)
            sleep(10)
        except NetworkError:
            # Can't reach etcd!
            # Step down immediately
            demote_to_replica()
            break
```

**2. Quorum Requirement**
```
etcd cluster needs majority (quorum) to operate:
- 3 etcd nodes: need 2 alive
- 5 etcd nodes: need 3 alive

If network partition:
- Side with quorum: can elect leader
- Side without quorum: can't elect leader
```

**3. Fencing**
```python
def try_become_leader():
    # Can only become leader if can write to etcd
    try:
        success = etcd.put('/db/pgcluster/leader',
                          value=my_name,
                          ttl=30,
                          prevExist=False)
        if success:
            promote_to_primary()
    except NoQuorum:
        # Can't reach etcd quorum
        # Stay as replica
        remain_replica()
```

---

## 8. Recovery Process

### 8.1 Node Recovery After Crash

```
Scenario: pg1 (leader) crashes and comes back

T=0s    pg1 crashes
T=15s   pg2 becomes new leader (Timeline 2)
T=60s   pg1 comes back online

Recovery Process:
┌─────────────────────────────────────────┐
│ 1. Patroni starts on pg1                │
│    ↓                                    │
│ 2. Check cluster state in etcd          │
│    - Current leader: pg2                │
│    - Current timeline: 2                │
│    - My last timeline: 1                │
│    ↓                                    │
│ 3. Detect timeline divergence           │
│    ↓                                    │
│ 4. Run pg_rewind                        │
│    - Rewind data to match new timeline  │
│    - Fetch missing WAL from pg2         │
│    ↓                                    │
│ 5. Configure as replica                 │
│    - Create recovery.signal             │
│    - Set primary_conninfo to pg2        │
│    ↓                                    │
│ 6. Start PostgreSQL in standby mode     │
│    ↓                                    │
│ 7. Start streaming from pg2             │
│    ↓                                    │
│ 8. Catch up with current data           │
│    ↓                                    │
│ 9. Join cluster as healthy replica      │
└─────────────────────────────────────────┘
```

### 8.2 pg_rewind Mechanism

```bash
# What pg_rewind does:
# 1. Compare data files between old primary and new primary
# 2. Copy only the changed blocks
# 3. Much faster than full re-sync

pg_rewind \
  --target-pgdata=/home/postgres/data \
  --source-server="host=pg2 port=5432 user=postgres"
```

**Configuration:**
```yaml
bootstrap:
  dcs:
    postgresql:
      use_pg_rewind: true  # Enable automatic pg_rewind
```

---

## 9. Timeline Management

### 9.1 What is Timeline?

```
Timeline = History of the cluster

Timeline 1: Initial cluster
  pg1 (primary) → pg2, pg3 (replicas)

Timeline 2: After first failover
  pg2 (primary) → pg1, pg3 (replicas)

Timeline 3: After second failover
  pg3 (primary) → pg1, pg2 (replicas)
```

### 9.2 Timeline in Action

```sql
-- Check current timeline
SELECT timeline_id FROM pg_control_checkpoint();

 timeline_id
-------------
           2

-- Timeline history
SELECT * FROM pg_stat_replication;

 timeline | sent_lsn  | write_lsn | flush_lsn | replay_lsn
----------+-----------+-----------+-----------+-----------
        2 | 0/4000060 | 0/4000060 | 0/4000060 | 0/4000060
```

**Timeline Files:**
```bash
/home/postgres/data/pg_wal/
├── 00000001.history  # Timeline 1 history
├── 00000002.history  # Timeline 2 history
└── 000000020000000000000001  # WAL file for timeline 2
```

---

## 10. Skenario Real-World

### Skenario 1: Leader Crash (Hardware Failure)

```
Initial State:
- pg1: Leader, Timeline 1
- pg2: Replica, streaming
- pg3: Replica, streaming

Event: pg1 server crashes (power failure)

T+0s:   pg1 goes offline
T+10s:  pg2 & pg3 detect leader is down
T+15s:  pg2 wins election, promotes to primary
T+20s:  pg3 reconfigures to follow pg2
T+25s:  HAProxy detects pg2 as new primary
T+30s:  Applications continue working (via HAProxy)

Result:
- pg2: Leader, Timeline 2
- pg3: Replica, streaming from pg2
- pg1: Offline
- Downtime: ~25 seconds
- Data loss: ZERO (synchronous replication)
```

### Skenario 2: Network Partition

```
Initial State:
- pg1: Leader
- pg2: Replica
- pg3: Replica
- etcd: 3 nodes (etcd1, etcd2, etcd3)

Event: Network partition isolates pg1

Partition A: pg1, etcd1
Partition B: pg2, pg3, etcd2, etcd3

What Happens:

Partition A (Minority):
- pg1 can't reach etcd quorum
- pg1 can't renew leader lock
- pg1 demotes itself to replica
- pg1 stops accepting writes

Partition B (Majority):
- pg2 & pg3 can reach etcd quorum
- pg2 wins election
- pg2 becomes new leader
- pg3 follows pg2
- Applications continue via HAProxy

Result: NO SPLIT-BRAIN!
- Only one leader (pg2)
- pg1 safely demoted
- Data consistency maintained
```

### Skenario 3: Planned Maintenance (Switchover)

```
Goal: Upgrade pg1 (current leader) without downtime

Steps:
1. Initiate switchover to pg2
   $ patronictl switchover --candidate node2

2. Patroni performs graceful switchover:
   - Stop new connections to pg1
   - Wait for pg2 to catch up (lag = 0)
   - Promote pg2 to primary
   - Demote pg1 to replica
   - HAProxy switches to pg2

3. Perform maintenance on pg1
   - Upgrade OS
   - Upgrade PostgreSQL
   - Hardware maintenance

4. Bring pg1 back as replica

5. (Optional) Switchover back to pg1

Result:
- Zero data loss
- Minimal downtime (~5 seconds)
- Controlled process
```

### Skenario 4: Replica Lag

```
Situation: pg3 has high replication lag

Monitoring:
$ patronictl list
| Member | Host | Role    | State     | Lag in MB |
|--------|------|---------|-----------|-----------|
| node1  | pg1  | Leader  | running   |           |
| node2  | pg2  | Replica | streaming | 0         |
| node3  | pg3  | Replica | streaming | 500       |

Causes:
- Slow disk I/O on pg3
- Network congestion
- Heavy query load on pg3

Patroni Actions:
1. Mark pg3 as "lagging"
2. Exclude pg3 from failover candidates
3. Alert monitoring system
4. If lag > threshold, stop replication

Resolution:
1. Investigate root cause
2. Fix performance issue
3. Let pg3 catch up
4. Re-enable as failover candidate
```

---

## Summary

### Key Takeaways

1. **Patroni = Brain of HA**
   - Monitors PostgreSQL health
   - Coordinates failover
   - Manages cluster state

2. **etcd = Consensus Layer**
   - Distributed leader election
   - Prevents split-brain
   - Stores cluster state

3. **PostgreSQL = Data Layer**
   - Streaming replication
   - WAL-based consistency
   - Timeline management

4. **HAProxy = Traffic Router**
   - Routes to current leader
   - Health checking
   - Connection pooling

### Performance Characteristics

| Metric | Value |
|--------|-------|
| Failover Time | < 30 seconds |
| Data Loss | Zero (sync replication) |
| Replication Lag | < 1 second (normal) |
| Health Check Interval | 10 seconds |
| Leader Lock TTL | 30 seconds |
| Split-Brain Risk | Prevented by etcd quorum |

### Best Practices

1. **Always use odd number of etcd nodes** (3, 5, 7)
2. **Monitor replication lag** continuously
3. **Test failover regularly** (chaos engineering)
4. **Use synchronous replication** for zero data loss
5. **Deploy across availability zones** for disaster recovery
6. **Set up proper monitoring** (Prometheus, Grafana)
7. **Have runbooks** for common scenarios
8. **Regular backups** (pg_basebackup, WAL archiving)

---

## Further Reading

- [Patroni Documentation](https://patroni.readthedocs.io/)
- [PostgreSQL Replication](https://www.postgresql.org/docs/current/high-availability.html)
- [etcd Documentation](https://etcd.io/docs/)
- [HAProxy Documentation](http://www.haproxy.org/)
