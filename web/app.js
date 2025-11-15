// State
let testRunning = false;
let testInterval = null;
let statsInterval = null;
let startTime = null;

const stats = {
    writeTotal: 0,
    writeSuccess: 0,
    writeFailed: 0,
    readTotal: 0,
    readSuccess: 0,
    readFailed: 0,
    reconnects: 0,
    currentNode: null
};

// Initialize
document.addEventListener('DOMContentLoaded', () => {
    // Initial stats display - show 0 ops, no availability yet
    updateStatsDisplay();
    
    // Set initial availability to show it's calculated from real data
    document.getElementById('availability').textContent = 'No data';
    document.getElementById('availability').style.color = '#8b949e';
    
    // Load cluster status
    loadClusterStatus();
    setInterval(loadClusterStatus, 5000); // Update every 5 seconds
    
    addLog('System initialized', 'info');
    addLog('Waiting for test to start...', 'info');
    addLog('Availability will be calculated from actual operations', 'info');
});

// Load cluster status
async function loadClusterStatus() {
    try {
        const response = await fetch('/api/cluster/status');
        const data = await response.json();
        
        updateClusterStatus(data);
        updateNodes(data.nodes);
    } catch (error) {
        console.error('Error loading cluster status:', error);
        addLog('Failed to load cluster status', 'error');
    }
}

// Update cluster status display
function updateClusterStatus(data) {
    document.getElementById('cluster-state').textContent = data.state || 'Unknown';
    document.getElementById('cluster-timeline').textContent = data.timeline || '-';
    
    if (startTime) {
        const uptime = Math.floor((Date.now() - startTime) / 1000);
        document.getElementById('cluster-uptime').textContent = formatUptime(uptime);
    }
}

// Update nodes display
function updateNodes(nodes) {
    const container = document.getElementById('nodes-container');
    container.innerHTML = '';
    
    if (!nodes || nodes.length === 0) {
        container.innerHTML = '<p>No nodes available</p>';
        return;
    }
    
    nodes.forEach(node => {
        const nodeDiv = document.createElement('div');
        nodeDiv.className = `node ${node.role.toLowerCase()}`;
        
        const statusClass = node.state === 'running' ? 'status-online' : 
                           node.state === 'stopped' ? 'status-offline' : 'status-warning';
        
        const roleClass = node.role === 'Leader' ? 'role-leader' : 
                         node.role === 'Replica' ? 'role-replica' : 'role-offline';
        
        nodeDiv.innerHTML = `
            <div class="node-header">
                <div class="node-name">
                    <span class="status-indicator ${statusClass}"></span>
                    ${node.name} (${node.host})
                </div>
                <span class="node-role ${roleClass}">${node.role}</span>
            </div>
            <div class="stat">
                <span class="stat-label">State:</span>
                <span class="stat-value">${node.state}</span>
            </div>
            <div class="stat">
                <span class="stat-label">Timeline:</span>
                <span class="stat-value">${node.timeline || '-'}</span>
            </div>
            ${node.lag !== undefined ? `
            <div class="stat">
                <span class="stat-label">Lag:</span>
                <span class="stat-value">${node.lag} MB</span>
            </div>
            ` : ''}
        `;
        
        container.appendChild(nodeDiv);
    });
}

// Start test
async function startTest() {
    if (testRunning) return;
    
    testRunning = true;
    startTime = Date.now();
    document.getElementById('start-btn').disabled = true;
    document.getElementById('stop-btn').disabled = false;
    
    // Reset stats for fresh start
    stats.writeTotal = 0;
    stats.writeSuccess = 0;
    stats.writeFailed = 0;
    stats.readTotal = 0;
    stats.readSuccess = 0;
    stats.readFailed = 0;
    stats.reconnects = 0;
    stats.currentNode = null;
    
    // Update display immediately
    updateStatsDisplay();
    
    showAlert('Test started! Writing and reading data every 2 seconds...', 'success');
    addLog('Test started - stats reset', 'success');
    
    // Start operations
    testInterval = setInterval(async () => {
        await performWrite();
        await performRead();
    }, 2000);
    
    // Update stats display
    statsInterval = setInterval(updateStatsDisplay, 1000);
}

// Stop test
function stopTest() {
    if (!testRunning) return;
    
    testRunning = false;
    document.getElementById('start-btn').disabled = false;
    document.getElementById('stop-btn').disabled = true;
    
    if (testInterval) {
        clearInterval(testInterval);
        testInterval = null;
    }
    
    if (statsInterval) {
        clearInterval(statsInterval);
        statsInterval = null;
    }
    
    showAlert('Test stopped', 'info');
    addLog('Test stopped', 'info');
}

// Perform write operation
async function performWrite() {
    stats.writeTotal++;
    const startTime = Date.now();
    
    try {
        const response = await fetch('/api/db/write', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                message: `Test message at ${new Date().toLocaleTimeString()}`
            })
        });
        
        const data = await response.json();
        const duration = Date.now() - startTime;
        
        if (data.success) {
            stats.writeSuccess++;
            addLog(`✓ Write OK (${duration}ms) → ${data.node}`, 'success');
            
            // Check for failover
            if (stats.currentNode && stats.currentNode !== data.node) {
                showAlert(`🔄 FAILOVER DETECTED! ${stats.currentNode} → ${data.node}`, 'warning');
                addLog(`🔄 FAILOVER: ${stats.currentNode} → ${data.node}`, 'warning');
                stats.reconnects++;
                
                // Calculate availability after failover
                const totalOps = stats.writeTotal + stats.readTotal;
                const successOps = stats.writeSuccess + stats.readSuccess;
                const currentAvailability = ((successOps / totalOps) * 100).toFixed(2);
                addLog(`📊 Current Availability: ${currentAvailability}%`, 'info');
            }
            stats.currentNode = data.node;
        } else {
            stats.writeFailed++;
            addLog(`✗ Write FAILED: ${data.error}`, 'error');
            
            // Log availability impact
            const totalOps = stats.writeTotal + stats.readTotal;
            const successOps = stats.writeSuccess + stats.readSuccess;
            const currentAvailability = ((successOps / totalOps) * 100).toFixed(2);
            addLog(`📉 Availability dropped to ${currentAvailability}%`, 'error');
        }
    } catch (error) {
        stats.writeFailed++;
        addLog(`✗ Write ERROR: ${error.message}`, 'error');
        
        // Log availability impact
        const totalOps = stats.writeTotal + stats.readTotal;
        const successOps = stats.writeSuccess + stats.readSuccess;
        const currentAvailability = ((successOps / totalOps) * 100).toFixed(2);
        addLog(`📉 Availability: ${currentAvailability}% (connection error)`, 'error');
    }
}

// Perform read operation
async function performRead() {
    stats.readTotal++;
    const startTime = Date.now();
    
    try {
        const response = await fetch('/api/db/read');
        const data = await response.json();
        const duration = Date.now() - startTime;
        
        if (data.success) {
            stats.readSuccess++;
            addLog(`✓ Read OK (${duration}ms): ${data.count} records`, 'success');
        } else {
            stats.readFailed++;
            addLog(`✗ Read FAILED: ${data.error}`, 'error');
            
            // Log availability impact
            const totalOps = stats.writeTotal + stats.readTotal;
            const successOps = stats.writeSuccess + stats.readSuccess;
            const currentAvailability = ((successOps / totalOps) * 100).toFixed(2);
            addLog(`📉 Availability dropped to ${currentAvailability}%`, 'error');
        }
    } catch (error) {
        stats.readFailed++;
        addLog(`✗ Read ERROR: ${error.message}`, 'error');
        
        // Log availability impact
        const totalOps = stats.writeTotal + stats.readTotal;
        const successOps = stats.writeSuccess + stats.readSuccess;
        const currentAvailability = ((successOps / totalOps) * 100).toFixed(2);
        addLog(`📉 Availability: ${currentAvailability}% (connection error)`, 'error');
    }
}

// Simulate failover
async function simulateFailover() {
    if (!confirm('This will stop the current leader. Continue?')) {
        return;
    }
    
    document.getElementById('failover-btn').disabled = true;
    showAlert('Simulating failover... Stopping current leader...', 'warning');
    addLog('Initiating failover simulation', 'warning');
    
    try {
        const response = await fetch('/api/cluster/failover', {
            method: 'POST'
        });
        
        const data = await response.json();
        
        if (data.success) {
            showAlert(`Failover initiated! Stopped ${data.stoppedNode}`, 'warning');
            addLog(`Stopped leader: ${data.stoppedNode}`, 'warning');
            
            // Re-enable button after 30 seconds
            setTimeout(() => {
                document.getElementById('failover-btn').disabled = false;
            }, 30000);
        } else {
            showAlert(`Failover failed: ${data.error}`, 'error');
            document.getElementById('failover-btn').disabled = false;
        }
    } catch (error) {
        showAlert(`Failover error: ${error.message}`, 'error');
        document.getElementById('failover-btn').disabled = false;
    }
}

// Update stats display
function updateStatsDisplay() {
    // Write stats
    document.getElementById('write-total').textContent = stats.writeTotal;
    document.getElementById('write-success').textContent = stats.writeSuccess;
    document.getElementById('write-failed').textContent = stats.writeFailed;
    
    // Calculate write success rate
    let writeRate = '100.00';
    if (stats.writeTotal > 0) {
        writeRate = ((stats.writeSuccess / stats.writeTotal) * 100).toFixed(2);
    }
    document.getElementById('write-rate').textContent = writeRate + '%';
    
    // Read stats
    document.getElementById('read-total').textContent = stats.readTotal;
    document.getElementById('read-success').textContent = stats.readSuccess;
    document.getElementById('read-failed').textContent = stats.readFailed;
    
    // Calculate read success rate
    let readRate = '100.00';
    if (stats.readTotal > 0) {
        readRate = ((stats.readSuccess / stats.readTotal) * 100).toFixed(2);
    }
    document.getElementById('read-rate').textContent = readRate + '%';
    
    // Reconnect/Failover count
    document.getElementById('reconnect-count').textContent = stats.reconnects;
    
    // Current node
    document.getElementById('current-node').textContent = stats.currentNode || '-';
    
    // Total failed operations
    const totalFailed = stats.writeFailed + stats.readFailed;
    document.getElementById('total-failed').textContent = totalFailed;
    
    // Total operations
    const totalOps = stats.writeTotal + stats.readTotal;
    const successOps = stats.writeSuccess + stats.readSuccess;
    document.getElementById('total-ops').textContent = totalOps;
    
    // Calculate real-time availability rate
    const availabilityEl = document.getElementById('availability');
    
    if (totalOps === 0) {
        // No operations yet - show "No data"
        availabilityEl.textContent = 'No data';
        availabilityEl.style.color = '#8b949e';
    } else {
        // Calculate from actual operations
        const availabilityRate = ((successOps / totalOps) * 100).toFixed(2);
        const rate = parseFloat(availabilityRate);
        
        // Update availability display with real-time calculation
        availabilityEl.textContent = availabilityRate + '%';
        
        // Update availability color based on real-time rate
        if (rate >= 99.9) {
            availabilityEl.style.color = '#7ee787'; // Green - Excellent (99.9%+)
        } else if (rate >= 99.0) {
            availabilityEl.style.color = '#58a6ff'; // Blue - Very Good (99.0-99.9%)
        } else if (rate >= 95.0) {
            availabilityEl.style.color = '#f0883e'; // Orange - Acceptable (95.0-99.0%)
        } else {
            availabilityEl.style.color = '#f85149'; // Red - Poor (<95%)
        }
    }
}

// Add log entry
function addLog(message, type = 'info') {
    const logContainer = document.getElementById('activity-log');
    const entry = document.createElement('div');
    entry.className = `log-entry log-${type}`;
    
    const time = new Date().toLocaleTimeString();
    entry.innerHTML = `<span class="log-time">[${time}]</span> ${message}`;
    
    logContainer.insertBefore(entry, logContainer.firstChild);
    
    // Keep only last 50 entries
    while (logContainer.children.length > 50) {
        logContainer.removeChild(logContainer.lastChild);
    }
}

// Show alert
function showAlert(message, type = 'info') {
    const container = document.getElementById('alert-container');
    const alert = document.createElement('div');
    alert.className = `alert alert-${type} fade-in`;
    alert.textContent = message;
    
    container.appendChild(alert);
    
    // Remove after 5 seconds
    setTimeout(() => {
        alert.style.opacity = '0';
        setTimeout(() => {
            container.removeChild(alert);
        }, 300);
    }, 5000);
}

// Clear logs
function clearLogs() {
    document.getElementById('activity-log').innerHTML = '';
    addLog('Logs cleared', 'info');
}

// Format uptime
function formatUptime(seconds) {
    const hours = Math.floor(seconds / 3600);
    const minutes = Math.floor((seconds % 3600) / 60);
    const secs = seconds % 60;
    
    if (hours > 0) {
        return `${hours}h ${minutes}m ${secs}s`;
    } else if (minutes > 0) {
        return `${minutes}m ${secs}s`;
    } else {
        return `${secs}s`;
    }
}
