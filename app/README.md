# PostgreSQL HA Test Application

Aplikasi Go sederhana untuk testing PostgreSQL High Availability dengan automatic failover detection.

## Features

✅ **Continuous Operations**
- Write data setiap 2 detik
- Read data setiap 2 detik
- Monitor connection status

✅ **Failover Detection**
- Automatic detection saat node berubah
- Reconnection logic dengan retry
- Real-time notification

✅ **Statistics**
- Total operations (read/write)
- Success/failure rate
- Uptime tracking
- Availability percentage

✅ **Visual Feedback**
- Color-coded output
- Real-time statistics
- Failover alerts

## Prerequisites

- Go 1.21 or higher
- PostgreSQL HA cluster running (via docker-compose)
- HAProxy accessible on localhost:54320

## Installation

```bash
cd app
go mod download
go build -o postgres-ha-test .
```

## Usage

### Run Manually

```bash
cd app
./postgres-ha-test
```

### Run with Automated Test

```bash
# From project root
./test-app-ha.sh
```

This script will:
1. Check cluster status
2. Build and start the application
3. Monitor for 10 seconds
4. Simulate failover (stop current leader)
5. Monitor during failover (30 seconds)
6. Restart old leader
7. Monitor recovery (20 seconds)
8. Show final statistics

## How It Works

### Connection

```go
// Connect via HAProxy
connStr := "host=localhost port=54320 user=postgres password=admin_password dbname=postgres sslmode=disable"
```

### Write Operation

```go
// Insert data with current node info
INSERT INTO ha_test (message, node_name) VALUES ($1, $2)
```

### Failover Detection

```go
// Detect when node changes
if app.stats.CurrentNode != nodeName {
    // FAILOVER DETECTED!
    // Old node: 172.28.0.3
    // New node: 172.28.0.4
}
```

### Reconnection Logic

```go
// Automatic reconnection with retry
maxRetries := 5
for i := 0; i < maxRetries; i++ {
    if err := app.connect(connStr); err == nil {
        // Reconnected successfully!
        return
    }
    time.Sleep(2 * time.Second)
}
```

## Output Example

```
╔══════════════════════════════════════════════════════════════╗
║                                                              ║
║        PostgreSQL HA Test Application                       ║
║        Testing Automatic Failover                           ║
║                                                              ║
╚══════════════════════════════════════════════════════════════╝

🔌 Connecting to PostgreSQL via HAProxy...
✅ Connected successfully!

✅ Database initialized!

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
                    STATISTICS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⏱️  Uptime:           45s
🖥️  Current Node:     172.28.0.3
🔄 Reconnects:       1

📝 Total Writes:     22
   ✓ Successful:     22 (100.0%)
   ✗ Failed:         0 (0.0%)

📖 Total Reads:      22
   ✓ Successful:     22 (100.0%)
   ✗ Failed:         0 (0.0%)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🔄 FAILOVER DETECTED!
   Old node: 172.28.0.3
   New node: 172.28.0.4

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
                    STATISTICS
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
⏱️  Uptime:           1m30s
🖥️  Current Node:     172.28.0.4
🔄 Reconnects:       1

📝 Total Writes:     45
   ✓ Successful:     43 (95.6%)
   ✗ Failed:         2 (4.4%)

📖 Total Reads:      45
   ✓ Successful:     43 (95.6%)
   ✗ Failed:         2 (4.4%)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## Testing Scenarios

### 1. Normal Operation

```bash
# Just run the app
./postgres-ha-test

# Watch statistics update every 5 seconds
# Press Ctrl+C to stop
```


```
docker run --rm --network host postgres-ha-test &
APP_PID=$!
sleep 15
kill $APP_PID 2>/dev/null || true

```

### 2. Manual Failover Test

```bash
# Terminal 1: Run application
./postgres-ha-test

# Terminal 2: Stop current leader
docker stop pg1  # or pg2/pg3 depending on current leader

# Watch Terminal 1 for failover detection
```

### 3. Automated Test

```bash
# From project root
./test-app-ha.sh

# This will automatically:
# - Start application
# - Simulate failover
# - Monitor recovery
# - Show statistics
```

## Statistics Explained

### Uptime
Total time application has been running

### Current Node
IP address of PostgreSQL node currently connected to

### Reconnects
Number of times application reconnected to database

### Write Operations
- **Total**: All write attempts
- **Successful**: Writes that completed successfully
- **Failed**: Writes that failed (during failover)

### Read Operations
- **Total**: All read attempts
- **Successful**: Reads that completed successfully
- **Failed**: Reads that failed (during failover)

### Availability
Percentage of successful operations:
- **≥99.9%**: Excellent (minimal downtime)
- **≥99.0%**: Good (acceptable downtime)
- **<99.0%**: Needs improvement

## Expected Behavior During Failover

1. **Before Failover**
   - All operations successful (100%)
   - Connected to leader node

2. **During Failover** (~30 seconds)
   - Some operations fail (connection errors)
   - Application detects failure
   - Automatic reconnection attempts
   - Failover detection when node changes

3. **After Failover**
   - Connected to new leader
   - Operations resume normally
   - Success rate returns to 100%

## Typical Metrics

| Metric | Expected Value |
|--------|----------------|
| Failover Detection | Immediate (when node changes) |
| Failed Operations | 2-5 operations (~4-10 seconds) |
| Reconnection Time | < 10 seconds |
| Availability | > 99% |
| Data Loss | 0 (all writes committed) |

## Troubleshooting

### Cannot Connect

```bash
# Check if cluster is running
docker ps | grep pg

# Check HAProxy
docker ps | grep haproxy

# Check cluster status
docker exec -it pg1 patronictl -c /etc/patroni.yml list
```

### High Failure Rate

```bash
# Check replication lag
docker exec -it pg1 patronictl -c /etc/patroni.yml list

# Check logs
docker logs pg1
docker logs haproxy
```

### Application Crashes

```bash
# Check logs
cat app.log

# Verify database exists
docker exec pg1 psql -U postgres -c "\l"

# Verify table exists
docker exec pg1 psql -U postgres -c "\dt"
```

## Code Structure

```
app/
├── main.go           # Main application
├── go.mod            # Go dependencies
├── Dockerfile        # Docker build file
└── README.md         # This file
```

### Key Functions

- `connect()`: Establish database connection
- `initDB()`: Create table if not exists
- `workload()`: Continuous write/read operations
- `writeData()`: Insert data and detect failover
- `readData()`: Query data
- `tryReconnect()`: Reconnection logic with retry
- `monitor()`: Print statistics every 5 seconds
- `printStats()`: Display current statistics
- `printFinalStats()`: Display final summary

## Dependencies

```go
require (
    github.com/lib/pq v1.10.9        // PostgreSQL driver
    github.com/fatih/color v1.16.0   // Colored output
)
```

## Configuration

Edit connection string in `main.go`:

```go
connStr := "host=localhost port=54320 user=postgres password=admin_password dbname=postgres sslmode=disable"
```

Parameters:
- `host`: HAProxy host (default: localhost)
- `port`: HAProxy port (default: 54320)
- `user`: PostgreSQL user
- `password`: PostgreSQL password
- `dbname`: Database name
- `sslmode`: SSL mode (disable for local testing)

## Performance Tuning

### Connection Pool

```go
db.SetMaxOpenConns(10)      // Max open connections
db.SetMaxIdleConns(5)       // Max idle connections
db.SetConnMaxLifetime(5m)   // Connection lifetime
```

### Operation Interval

```go
ticker := time.NewTicker(2 * time.Second)  // Write/read every 2 seconds
```

### Monitoring Interval

```go
ticker := time.NewTicker(5 * time.Second)  // Print stats every 5 seconds
```

## License

MIT
