# Patroni Internals: Deep Dive

## Patroni Main Loop

Patroni berjalan dalam infinite loop yang mengecek dan mengelola cluster setiap `loop_wait` seconds (default: 10 detik).

### Main Loop Pseudocode

```python
class Patroni:
    def __init__(self):
        self.postgresql = PostgreSQL()
        self.dcs = etcd3.Client()  # Distributed Configuration Store
        self.config = load_config('/etc/patroni.yml')
        self.is_leader = False
        
    def run(self):
        """Main loop - runs forever"""
        while True:
            try:
                self.run_cycle()
            except Exception as e:
                logger.error(f"Error in main loop: {e}")
            
            time.sleep(self.config['loop_wait'])  # Default: 10 seconds
    
    def run_cycle(self):
        """Single iteration of the main loop"""
        
        # 1. Check PostgreSQL health
        pg_status = self.check_postgresql_health()
        if not pg_status.is_healthy:
            self.handle_unhealthy_postgresql()
            return
        
        # 2. Get cluster state from DCS (etcd)
        cluster_state = self.dcs.get_cluster_state()
        
        # 3. Try to acquire or renew leader lock
        if self.is_leader:
            # I am leader, try to renew lock
            if not self.renew_leader_lock():
                # Failed to renew! Step down
                self.demote_to_replica()
                self.is_leader = False
        else:
            # I am replica, check if I should become leader
            if cluster_state.leader is None:
                # No leader! Try to become leader
                if self.try_acquire_leader_lock():
                    self.promote_to_leader()
                    self.is_leader = True
        
        # 4. Update my status in DCS
        self.update_member_status()
        
        # 5. Check if I need to follow a new leader
        if not self.is_leader:
            if cluster_state.leader != self.current_primary:
                self.follow_new_leader(cluster_state.leader)
        
        # 6. Handle any pending operations
        self.handle_pending_operations()
        
        # 7. Update metrics and logs
        self.update_metrics()
```

---

## Leader Lock Mechanism

### How Leader Lock Works

```python
class LeaderLock:
    def __init__(self, dcs, cluster_name, node_name):
        self.dcs = dcs
        self.cluster_name = cluster_name
        self.node_name = node_name
        self.lock_key = f"/db/{cluster_name}/leader"
        self.ttl = 30  # seconds
        self.lease_id = None
    
    def try_acquire(self):
        """Try to acquire leader lock"""
        try:
            # Create a lease with TTL
            self.lease_id = self.dcs.lease_grant(self.ttl)
            
            # Try to put key with lease, only if key doesn't exist
            success = self.dcs.put_if_not_exists(
                key=self.lock_key,
                value=self.node_name,
                lease=self.lease_id
            )
            
            if success:
                logger.info(f"Acquired leader lock: {self.node_name}")
                return True
            else:
                logger.info(f"Failed to acquire lock, already held by another node")
                return False
                
        except Exception as e:
            logger.error(f"Error acquiring lock: {e}")
            return False
    
    def renew(self):
        """Renew leader lock"""
        try:
            # Refresh the lease
            self.dcs.lease_refresh(self.lease_id)
            logger.debug(f"Renewed leader lock: {self.node_name}")
            return True
        except Exception as e:
            logger.error(f"Failed to renew lock: {e}")
            return False
    
    def release(self):
        """Release leader lock"""
        try:
            self.dcs.delete(self.lock_key)
            if self.lease_id:
                self.dcs.lease_revoke(self.lease_id)
            logger.info(f"Released leader lock: {self.node_name}")
        except Exception as e:
            logger.error(f"Error releasing lock: {e}")
```

### Lock Lifecycle

```
Leader Node (pg1):
┌─────────────────────────────────────────────────────────┐
│ T=0s:   Acquire lock (TTL: 30s)                        │
│         lease_id = 12345                                │
│                                                         │
│ T=10s:  Renew lock (refresh lease)                     │
│         lease_id = 12345 (TTL reset to 30s)            │
│                                                         │
│ T=20s:  Renew lock (refresh lease)                     │
│         lease_id = 12345 (TTL reset to 30s)            │
│                                                         │
│ T=30s:  Renew lock (refresh lease)                     │
│         lease_id = 12345 (TTL reset to 30s)            │
│                                                         │
│ ... continues every 10s ...                            │
└─────────────────────────────────────────────────────────┘

If Leader Crashes:
┌─────────────────────────────────────────────────────────┐
│ T=0s:   Last successful renew                          │
│         lease_id = 12345 (TTL: 30s)                    │
│                                                         │
│ T=5s:   Leader crashes! 💥                             │
│                                                         │
│ T=10s:  Missed renew (leader is dead)                  │
│         TTL countdown: 20s remaining                    │
│                                                         │
│ T=20s:  Missed renew (leader still dead)               │
│         TTL countdown: 10s remaining                    │
│                                                         │
│ T=30s:  Missed renew (leader still dead)               │
│         TTL countdown: 0s remaining                     │
│         Lock EXPIRES! 🔓                                │
│                                                         │
│ T=35s:  Replica detects lock is gone                   │
│         Starts leader election                          │
└─────────────────────────────────────────────────────────┘
```

---

## PostgreSQL Health Checks

### Health Check Implementation

```python
class PostgreSQLHealthCheck:
    def __init__(self, postgresql):
        self.postgresql = postgresql
        self.connection = None
    
    def check_health(self):
        """Comprehensive health check"""
        checks = {
            'process_running': self.check_process(),
            'can_connect': self.check_connection(),
            'accepting_connections': self.check_accepting_connections(),
            'replication_healthy': self.check_replication(),
            'disk_space': self.check_disk_space(),
            'timeline_match': self.check_timeline()
        }
        
        # All checks must pass
        is_healthy = all(checks.values())
        
        return HealthStatus(
            is_healthy=is_healthy,
            checks=checks,
            timestamp=time.time()
        )
    
    def check_process(self):
        """Check if PostgreSQL process is running"""
        try:
            # Check postmaster.pid file
            pid_file = '/home/postgres/data/postmaster.pid'
            if not os.path.exists(pid_file):
                return False
            
            # Read PID from file
            with open(pid_file, 'r') as f:
                pid = int(f.readline().strip())
            
            # Check if process exists
            os.kill(pid, 0)  # Signal 0 = check if process exists
            return True
            
        except (OSError, ValueError):
            return False
    
    def check_connection(self):
        """Check if can connect to PostgreSQL"""
        try:
            self.connection = psycopg2.connect(
                host='localhost',
                port=5432,
                user='postgres',
                password='admin_password',
                database='postgres',
                connect_timeout=3
            )
            return True
        except psycopg2.Error:
            return False
    
    def check_accepting_connections(self):
        """Check if PostgreSQL is accepting connections"""
        try:
            cursor = self.connection.cursor()
            cursor.execute("SELECT 1")
            result = cursor.fetchone()
            return result[0] == 1
        except psycopg2.Error:
            return False
    
    def check_replication(self):
        """Check replication status"""
        try:
            cursor = self.connection.cursor()
            
            # Check if I'm primary or standby
            cursor.execute("SELECT pg_is_in_recovery()")
            is_standby = cursor.fetchone()[0]
            
            if is_standby:
                # I'm standby, check if receiving WAL
                cursor.execute("""
                    SELECT status, received_lsn, latest_end_lsn
                    FROM pg_stat_wal_receiver
                """)
                result = cursor.fetchone()
                
                if result is None:
                    return False  # Not receiving WAL!
                
                status = result[0]
                return status == 'streaming'
            else:
                # I'm primary, check if replicas are connected
                cursor.execute("""
                    SELECT count(*) 
                    FROM pg_stat_replication
                    WHERE state = 'streaming'
                """)
                replica_count = cursor.fetchone()[0]
                return replica_count > 0  # At least one replica
                
        except psycopg2.Error:
            return False
    
    def check_disk_space(self):
        """Check if enough disk space"""
        try:
            stat = os.statvfs('/home/postgres/data')
            free_bytes = stat.f_bavail * stat.f_frsize
            free_gb = free_bytes / (1024**3)
            
            # Need at least 1GB free
            return free_gb > 1.0
        except OSError:
            return False
    
    def check_timeline(self):
        """Check if timeline matches cluster"""
        try:
            cursor = self.connection.cursor()
            cursor.execute("SELECT timeline_id FROM pg_control_checkpoint()")
            my_timeline = cursor.fetchone()[0]
            
            # Get expected timeline from DCS
            cluster_state = self.dcs.get_cluster_state()
            expected_timeline = cluster_state.timeline
            
            return my_timeline == expected_timeline
        except (psycopg2.Error, AttributeError):
            return False
```

---

## Promotion Process

### Promote Replica to Primary

```python
class PromotionManager:
    def __init__(self, postgresql, dcs):
        self.postgresql = postgresql
        self.dcs = dcs
    
    def promote_to_primary(self):
        """Promote this replica to primary"""
        logger.info("Starting promotion to primary")
        
        try:
            # Step 1: Verify I have the leader lock
            if not self.verify_leader_lock():
                raise Exception("Don't have leader lock!")
            
            # Step 2: Wait for WAL replay to complete
            self.wait_for_wal_replay()
            
            # Step 3: Remove recovery.signal
            self.remove_recovery_signal()
            
            # Step 4: Update configuration
            self.update_postgresql_conf()
            
            # Step 5: Promote PostgreSQL
            self.execute_promotion()
            
            # Step 6: Wait for promotion to complete
            self.wait_for_promotion()
            
            # Step 7: Verify I'm now primary
            if not self.verify_is_primary():
                raise Exception("Promotion failed!")
            
            # Step 8: Update cluster state
            self.update_cluster_state()
            
            # Step 9: Create replication slots for replicas
            self.create_replication_slots()
            
            logger.info("Promotion completed successfully")
            return True
            
        except Exception as e:
            logger.error(f"Promotion failed: {e}")
            self.rollback_promotion()
            return False
    
    def wait_for_wal_replay(self):
        """Wait for all WAL to be replayed"""
        logger.info("Waiting for WAL replay to complete")
        
        timeout = 30  # seconds
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            cursor = self.postgresql.cursor()
            cursor.execute("""
                SELECT pg_wal_lsn_diff(
                    pg_last_wal_receive_lsn(),
                    pg_last_wal_replay_lsn()
                ) AS lag_bytes
            """)
            lag = cursor.fetchone()[0]
            
            if lag == 0:
                logger.info("WAL replay complete")
                return
            
            logger.debug(f"WAL replay lag: {lag} bytes")
            time.sleep(0.5)
        
        raise Exception("Timeout waiting for WAL replay")
    
    def remove_recovery_signal(self):
        """Remove recovery.signal file"""
        signal_file = '/home/postgres/data/recovery.signal'
        if os.path.exists(signal_file):
            os.remove(signal_file)
            logger.info("Removed recovery.signal")
    
    def update_postgresql_conf(self):
        """Update PostgreSQL configuration for primary mode"""
        config_updates = {
            'hot_standby': 'off',
            'wal_level': 'replica',
            'max_wal_senders': '10',
            'max_replication_slots': '10'
        }
        
        self.postgresql.update_config(config_updates)
        logger.info("Updated PostgreSQL configuration")
    
    def execute_promotion(self):
        """Execute pg_ctl promote"""
        logger.info("Executing pg_ctl promote")
        
        result = subprocess.run(
            ['pg_ctl', 'promote', '-D', '/home/postgres/data'],
            capture_output=True,
            text=True,
            timeout=30
        )
        
        if result.returncode != 0:
            raise Exception(f"pg_ctl promote failed: {result.stderr}")
        
        logger.info("pg_ctl promote executed successfully")
    
    def wait_for_promotion(self):
        """Wait for promotion to complete"""
        logger.info("Waiting for promotion to complete")
        
        timeout = 30
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            if self.verify_is_primary():
                logger.info("Promotion complete")
                return
            time.sleep(1)
        
        raise Exception("Timeout waiting for promotion")
    
    def verify_is_primary(self):
        """Verify this node is now primary"""
        try:
            cursor = self.postgresql.cursor()
            cursor.execute("SELECT pg_is_in_recovery()")
            is_in_recovery = cursor.fetchone()[0]
            return not is_in_recovery  # Primary = not in recovery
        except:
            return False
    
    def update_cluster_state(self):
        """Update cluster state in DCS"""
        cursor = self.postgresql.cursor()
        cursor.execute("SELECT timeline_id FROM pg_control_checkpoint()")
        timeline = cursor.fetchone()[0]
        
        state = {
            'role': 'master',
            'state': 'running',
            'timeline': timeline,
            'conn_url': f'postgresql://{self.postgresql.host}:5432',
            'api_url': f'http://{self.postgresql.host}:8008'
        }
        
        self.dcs.update_member_state(self.postgresql.name, state)
        logger.info(f"Updated cluster state: {state}")
    
    def create_replication_slots(self):
        """Create replication slots for replicas"""
        # Get list of replicas from cluster
        cluster_state = self.dcs.get_cluster_state()
        
        for member in cluster_state.members:
            if member.name != self.postgresql.name:
                slot_name = member.name
                
                try:
                    cursor = self.postgresql.cursor()
                    cursor.execute(f"""
                        SELECT pg_create_physical_replication_slot('{slot_name}')
                    """)
                    logger.info(f"Created replication slot: {slot_name}")
                except psycopg2.errors.DuplicateObject:
                    logger.debug(f"Replication slot already exists: {slot_name}")
```

---

## Replica Reconfiguration

### Follow New Leader

```python
class ReplicaManager:
    def __init__(self, postgresql, dcs):
        self.postgresql = postgresql
        self.dcs = dcs
    
    def follow_new_leader(self, new_leader_name):
        """Reconfigure to follow new leader"""
        logger.info(f"Reconfiguring to follow new leader: {new_leader_name}")
        
        try:
            # Step 1: Get new leader info from DCS
            leader_info = self.dcs.get_member_info(new_leader_name)
            if not leader_info:
                raise Exception(f"Leader info not found: {new_leader_name}")
            
            # Step 2: Check if timeline diverged
            if self.check_timeline_divergence(leader_info):
                # Need to rewind
                self.perform_rewind(leader_info)
            
            # Step 3: Stop PostgreSQL
            self.postgresql.stop()
            
            # Step 4: Update replication configuration
            self.update_replication_config(leader_info)
            
            # Step 5: Create recovery.signal
            self.create_recovery_signal()
            
            # Step 6: Start PostgreSQL in standby mode
            self.postgresql.start()
            
            # Step 7: Wait for replication to start
            self.wait_for_replication()
            
            logger.info("Successfully reconfigured to follow new leader")
            return True
            
        except Exception as e:
            logger.error(f"Failed to follow new leader: {e}")
            return False
    
    def check_timeline_divergence(self, leader_info):
        """Check if our timeline diverged from leader"""
        cursor = self.postgresql.cursor()
        cursor.execute("SELECT timeline_id FROM pg_control_checkpoint()")
        my_timeline = cursor.fetchone()[0]
        
        leader_timeline = leader_info['timeline']
        
        if my_timeline != leader_timeline:
            logger.warning(f"Timeline divergence detected: "
                         f"my={my_timeline}, leader={leader_timeline}")
            return True
        return False
    
    def perform_rewind(self, leader_info):
        """Perform pg_rewind to sync with new leader"""
        logger.info("Performing pg_rewind")
        
        # Stop PostgreSQL first
        self.postgresql.stop()
        
        # Run pg_rewind
        result = subprocess.run([
            'pg_rewind',
            '--target-pgdata=/home/postgres/data',
            f'--source-server=host={leader_info["host"]} '
            f'port=5432 user=postgres password=admin_password'
        ], capture_output=True, text=True, timeout=300)
        
        if result.returncode != 0:
            raise Exception(f"pg_rewind failed: {result.stderr}")
        
        logger.info("pg_rewind completed successfully")
    
    def update_replication_config(self, leader_info):
        """Update replication configuration"""
        primary_conninfo = (
            f"host={leader_info['host']} "
            f"port=5432 "
            f"user=replicator "
            f"password=rep_password "
            f"application_name={self.postgresql.name}"
        )
        
        config_updates = {
            'primary_conninfo': primary_conninfo,
            'primary_slot_name': self.postgresql.name,
            'hot_standby': 'on',
            'hot_standby_feedback': 'on'
        }
        
        self.postgresql.update_config(config_updates)
        logger.info(f"Updated replication config: {primary_conninfo}")
    
    def create_recovery_signal(self):
        """Create recovery.signal file"""
        signal_file = '/home/postgres/data/recovery.signal'
        with open(signal_file, 'w') as f:
            f.write('')
        logger.info("Created recovery.signal")
    
    def wait_for_replication(self):
        """Wait for replication to start"""
        logger.info("Waiting for replication to start")
        
        timeout = 60
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            try:
                cursor = self.postgresql.cursor()
                cursor.execute("""
                    SELECT status FROM pg_stat_wal_receiver
                """)
                result = cursor.fetchone()
                
                if result and result[0] == 'streaming':
                    logger.info("Replication started successfully")
                    return True
                    
            except psycopg2.Error:
                pass
            
            time.sleep(2)
        
        raise Exception("Timeout waiting for replication to start")
```

---

## REST API Implementation

```python
from flask import Flask, jsonify, request

class PatroniAPI:
    def __init__(self, patroni):
        self.patroni = patroni
        self.app = Flask(__name__)
        self.setup_routes()
    
    def setup_routes(self):
        """Setup REST API routes"""
        
        @self.app.route('/health', methods=['GET'])
        def health():
            """Overall health check"""
            health_status = self.patroni.get_health_status()
            
            if health_status.is_healthy:
                return jsonify({
                    'state': 'running',
                    'role': health_status.role,
                    'timeline': health_status.timeline
                }), 200
            else:
                return jsonify({
                    'state': 'unhealthy',
                    'errors': health_status.errors
                }), 503
        
        @self.app.route('/leader', methods=['GET'])
        @self.app.route('/primary', methods=['GET'])
        def leader():
            """Check if this node is leader"""
            if self.patroni.is_leader:
                return jsonify({
                    'state': 'running',
                    'role': 'master',
                    'timeline': self.patroni.get_timeline()
                }), 200
            else:
                return jsonify({
                    'state': 'running',
                    'role': 'replica'
                }), 503
        
        @self.app.route('/replica', methods=['GET'])
        def replica():
            """Check if this node is replica"""
            if not self.patroni.is_leader:
                lag = self.patroni.get_replication_lag()
                return jsonify({
                    'state': 'running',
                    'role': 'replica',
                    'lag': lag
                }), 200
            else:
                return jsonify({
                    'state': 'running',
                    'role': 'master'
                }), 503
        
        @self.app.route('/patroni', methods=['GET'])
        def patroni_status():
            """Full Patroni status"""
            return jsonify({
                'state': self.patroni.state,
                'role': self.patroni.role,
                'server_version': self.patroni.get_pg_version(),
                'timeline': self.patroni.get_timeline(),
                'xlog': {
                    'location': self.patroni.get_xlog_location()
                },
                'patroni': {
                    'version': '3.3.0',
                    'scope': self.patroni.scope,
                    'name': self.patroni.name
                }
            }), 200
    
    def run(self, host='0.0.0.0', port=8008):
        """Start REST API server"""
        self.app.run(host=host, port=port, threaded=True)
```

---

## Summary

Patroni adalah orchestrator yang kompleks dengan banyak komponen:

1. **Main Loop**: Runs every 10 seconds, checks health, manages locks
2. **Leader Lock**: Distributed lock in etcd with TTL, prevents split-brain
3. **Health Checks**: Comprehensive checks of PostgreSQL status
4. **Promotion**: Complex process to promote replica to primary
5. **Reconfiguration**: Automatic reconfiguration to follow new leader
6. **REST API**: Exposes status for monitoring and HAProxy

Semua komponen bekerja sama untuk memberikan automatic failover dengan zero data loss!
