# PostgreSQL HA Web UI

Interactive web interface untuk monitoring dan testing PostgreSQL High Availability cluster.

## Features

### 📊 Real-time Monitoring
- **Cluster Status**: Live cluster state, timeline, dan uptime
- **Node Status**: Status setiap PostgreSQL node (Leader/Replica)
- **Visual Indicators**: Color-coded status dengan animasi
- **Auto-refresh**: Update otomatis setiap 5 detik

### 📈 Operations Tracking
- **Write Operations**: Track semua write operations
- **Read Operations**: Track semua read operations
- **Success Rate**: Real-time success rate percentage
- **Availability**: Overall availability metrics

### 🎮 Interactive Controls
- **Start/Stop Test**: Mulai/stop continuous testing
- **Simulate Failover**: Trigger failover dengan satu klik
- **Clear Logs**: Bersihkan activity log

### 📋 Activity Log
- **Real-time Logging**: Semua operasi ter-log dengan timestamp
- **Color-coded**: Success (green), Error (red), Warning (yellow), Info (blue)
- **Auto-scroll**: Log terbaru di atas

### 🎯 Failover Detection
- **Automatic Detection**: Detect saat node berubah
- **Visual Alert**: Alert banner saat failover terjadi
- **Node Tracking**: Track node mana yang sedang aktif

## Quick Start

### Run with Script

```bash
# From project root
./run-web-ui.sh
```

### Manual Run

```bash
# Build
cd web
docker build -t postgres-ha-web .

# Run
docker run --rm \
    --network host \
    -v /var/run/docker.sock:/var/run/docker.sock \
    postgres-ha-web
```

### Access

Open browser: **http://localhost:8080**

## How to Use

### 1. Start Monitoring

- Web UI akan otomatis load cluster status
- Lihat status semua nodes (Leader/Replica)
- Monitor cluster timeline dan state

### 2. Start Test

1. Click **▶️ Start Test** button
2. Aplikasi akan mulai write/read setiap 2 detik
3. Watch statistics update real-time
4. Monitor success rate dan availability

### 3. Simulate Failover

1. Click **💥 Simulate Failover** button
2. Confirm dialog
3. Current leader akan di-stop
4. Watch untuk:
   - Failed operations (2-5 ops)
   - Failover detection alert
   - New leader election
   - Operations resume
5. Old leader akan restart otomatis setelah 30 detik

### 4. Monitor Results

- **Success Rate**: Should return to 100% after failover
- **Availability**: Should stay > 99%
- **Activity Log**: See all operations and events
- **Node Status**: See which node is current leader

## UI Components

### Header
- Title dan description
- Always visible

### Cluster Status Card
- Current cluster state
- Timeline number
- Uptime counter

### Operations Card
- Total operations count
- Success rate progress bar
- Visual percentage

### Availability Card
- Large percentage display
- Color-coded (green/yellow/red)

### Nodes Card
- List of all PostgreSQL nodes
- Each node shows:
  - Name dan host
  - Role (Leader/Replica)
  - State (running/stopped)
  - Timeline
  - Replication lag (for replicas)
- Color-coded borders:
  - Green: Leader
  - Blue: Replica
  - Red: Offline

### Controls Card
- Start/Stop test buttons
- Simulate failover button
- Clear logs button
- Alert messages

### Statistics Cards
- Write operations (total/success/failed)
- Read operations (total/success/failed)

### Activity Log
- Scrollable log area
- Color-coded entries
- Timestamps
- Auto-scroll to latest

## API Endpoints

### GET /api/cluster/status
Get cluster status dan node information

Response:
```json
{
  "state": "Running",
  "timeline": 5,
  "nodes": [
    {
      "name": "node1",
      "host": "pg1",
      "role": "Leader",
      "state": "running",
      "timeline": 5,
      "lag": 0
    }
  ]
}
```

### POST /api/db/write
Write data to database

Request:
```json
{
  "message": "Test message"
}
```

Response:
```json
{
  "success": true,
  "message": "Write successful",
  "node": "172.28.0.3"
}
```

### GET /api/db/read
Read data from database

Response:
```json
{
  "success": true,
  "count": 42
}
```

### POST /api/cluster/failover
Simulate failover (stop current leader)

Response:
```json
{
  "success": true,
  "message": "Stopped pg1, will restart in 30 seconds",
  "node": "pg1"
}
```

## Architecture

```
┌─────────────┐
│   Browser   │
│  (Client)   │
└──────┬──────┘
       │ HTTP
       ▼
┌─────────────┐
│  Web Server │
│   (Go)      │
│   :8080     │
└──────┬──────┘
       │
       ├─────────────┐
       │             │
       ▼             ▼
┌─────────────┐ ┌─────────────┐
│  PostgreSQL │ │   Docker    │
│  via HAProxy│ │   Commands  │
│   :54320    │ │             │
└─────────────┘ └─────────────┘
```

## Technology Stack

### Frontend
- **HTML5**: Structure
- **CSS3**: Styling dengan gradients dan animations
- **JavaScript**: Logic dan API calls
- **Fetch API**: HTTP requests

### Backend
- **Go**: Web server
- **net/http**: HTTP server
- **database/sql**: PostgreSQL driver
- **os/exec**: Docker commands

### Database
- **PostgreSQL**: Via HAProxy
- **Connection Pool**: 10 max connections

## Configuration

### Server Port
Default: 8080

Change in `server.go`:
```go
http.ListenAndServe(":8080", nil)
```

### Database Connection
Default: localhost:54320

Change in `server.go`:
```go
connStr := "host=localhost port=54320 user=postgres password=admin_password dbname=postgres sslmode=disable"
```

### Operation Interval
Default: 2 seconds

Change in `app.js`:
```javascript
testInterval = setInterval(async () => {
    await performWrite();
    await performRead();
}, 2000); // 2000ms = 2 seconds
```

### Status Refresh
Default: 5 seconds

Change in `app.js`:
```javascript
setInterval(loadClusterStatus, 5000); // 5000ms = 5 seconds
```

## Troubleshooting

### Cannot Connect to Server

```bash
# Check if server is running
docker ps | grep postgres-ha-web

# Check logs
docker logs <container-id>
```

### Cannot Connect to Database

```bash
# Check cluster status
docker exec -it pg1 patronictl -c /etc/patroni.yml list

# Check HAProxy
docker ps | grep haproxy
```

### Failover Button Not Working

```bash
# Check docker socket is mounted
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock alpine ls -la /var/run/docker.sock

# Check permissions
ls -la /var/run/docker.sock
```

### UI Not Updating

- Check browser console for errors (F12)
- Check network tab for failed API calls
- Refresh page (Ctrl+R)

## Development

### Run Locally (without Docker)

```bash
cd web

# Install dependencies
go mod download

# Run server
go run server.go
```

### Build

```bash
cd web
docker build -t postgres-ha-web .
```

### Test API

```bash
# Get cluster status
curl http://localhost:8080/api/cluster/status

# Write data
curl -X POST http://localhost:8080/api/db/write \
  -H "Content-Type: application/json" \
  -d '{"message":"test"}'

# Read data
curl http://localhost:8080/api/db/read

# Simulate failover
curl -X POST http://localhost:8080/api/cluster/failover
```

## Screenshots

### Normal Operation
- All nodes green
- 100% success rate
- Operations running smoothly

### During Failover
- Leader node turns red
- Some operations fail
- Alert banner appears
- New leader elected

### After Failover
- New leader is green
- Success rate returns to 100%
- Old leader rejoins as replica

## Performance

- **Page Load**: < 1 second
- **API Response**: < 100ms
- **UI Update**: Real-time (no lag)
- **Memory Usage**: < 50MB
- **CPU Usage**: < 5%

## Security Notes

⚠️ **This is for testing/demo purposes only!**

For production:
- Add authentication
- Use HTTPS
- Validate all inputs
- Rate limiting
- CORS configuration
- Secure database credentials

## License

MIT
