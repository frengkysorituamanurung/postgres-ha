package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

type Server struct {
	db *sql.DB
}

type ClusterStatus struct {
	State    string `json:"state"`
	Timeline int    `json:"timeline"`
	Nodes    []Node `json:"nodes"`
}

type Node struct {
	Name     string `json:"name"`
	Host     string `json:"host"`
	Role     string `json:"role"`
	State    string `json:"state"`
	Timeline int    `json:"timeline"`
	Lag      int    `json:"lag"`
}

type WriteRequest struct {
	Message string `json:"message"`
}

type Response struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
	Node    string `json:"node,omitempty"`
	Count   int    `json:"count,omitempty"`
}

func main() {
	server := &Server{}
	
	// Connect to database
	connStr := "host=localhost port=54320 user=postgres password=admin_password dbname=postgres sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()
	
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)
	
	if err := db.Ping(); err != nil {
		log.Fatal("Failed to ping database:", err)
	}
	
	server.db = db
	
	// Initialize database
	if err := server.initDB(); err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	
	// Setup routes
	http.HandleFunc("/", server.serveIndex)
	http.HandleFunc("/app.js", server.serveJS)
	http.HandleFunc("/api/cluster/status", server.handleClusterStatus)
	http.HandleFunc("/api/cluster/failover", server.handleFailover)
	http.HandleFunc("/api/db/write", server.handleWrite)
	http.HandleFunc("/api/db/read", server.handleRead)
	
	fmt.Println("🚀 Server starting on http://localhost:8080")
	fmt.Println("📊 Open http://localhost:8080 in your browser")
	
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Server failed:", err)
	}
}

func (s *Server) initDB() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS ha_test (
			id SERIAL PRIMARY KEY,
			message TEXT NOT NULL,
			node_name TEXT,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	return err
}

func (s *Server) serveIndex(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func (s *Server) serveJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	http.ServeFile(w, r, "app.js")
}

func (s *Server) handleClusterStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	// Get cluster status from patronictl
	cmd := exec.Command("docker", "exec", "pg1", "patronictl", "-c", "/etc/patroni.yml", "list", "--format", "json")
	output, err := cmd.Output()
	
	if err != nil {
		// Try pg2 if pg1 fails
		cmd = exec.Command("docker", "exec", "pg2", "patronictl", "-c", "/etc/patroni.yml", "list", "--format", "json")
		output, err = cmd.Output()
		
		if err != nil {
			// Try pg3 if pg2 fails
			cmd = exec.Command("docker", "exec", "pg3", "patronictl", "-c", "/etc/patroni.yml", "list", "--format", "json")
			output, err = cmd.Output()
		}
	}
	
	if err != nil {
		json.NewEncoder(w).Encode(ClusterStatus{
			State: "Error",
			Nodes: []Node{},
		})
		return
	}
	
	// Parse patronictl output
	var patroniNodes []map[string]interface{}
	if err := json.Unmarshal(output, &patroniNodes); err != nil {
		log.Println("Failed to parse patronictl output:", err)
		json.NewEncoder(w).Encode(ClusterStatus{
			State: "Error",
			Nodes: []Node{},
		})
		return
	}
	
	// Convert to our Node format
	nodes := make([]Node, 0)
	maxTimeline := 0
	
	for _, pn := range patroniNodes {
		node := Node{
			Name:  getString(pn, "Member"),
			Host:  getString(pn, "Host"),
			Role:  getString(pn, "Role"),
			State: getString(pn, "State"),
		}
		
		if tl, ok := pn["TL"].(float64); ok {
			node.Timeline = int(tl)
			if node.Timeline > maxTimeline {
				maxTimeline = node.Timeline
			}
		}
		
		if lag, ok := pn["Lag in MB"].(float64); ok {
			node.Lag = int(lag)
		}
		
		nodes = append(nodes, node)
	}
	
	status := ClusterStatus{
		State:    "Running",
		Timeline: maxTimeline,
		Nodes:    nodes,
	}
	
	json.NewEncoder(w).Encode(status)
}

func (s *Server) handleWrite(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}
	
	var req WriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   "Invalid request",
		})
		return
	}
	
	// Get current node
	var nodeName string
	err := s.db.QueryRow("SELECT inet_server_addr()").Scan(&nodeName)
	if err != nil {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	
	// Insert data
	_, err = s.db.Exec(
		"INSERT INTO ha_test (message, node_name) VALUES ($1, $2)",
		req.Message, nodeName,
	)
	
	if err != nil {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Message: "Write successful",
		Node:    nodeName,
	})
}

func (s *Server) handleRead(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM ha_test").Scan(&count)
	
	if err != nil {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   err.Error(),
		})
		return
	}
	
	json.NewEncoder(w).Encode(Response{
		Success: true,
		Count:   count,
	})
}

func (s *Server) handleFailover(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   "Method not allowed",
		})
		return
	}
	
	// Get current leader
	cmd := exec.Command("docker", "exec", "pg1", "patronictl", "-c", "/etc/patroni.yml", "list")
	output, err := cmd.Output()
	
	if err != nil {
		cmd = exec.Command("docker", "exec", "pg2", "patronictl", "-c", "/etc/patroni.yml", "list")
		output, err = cmd.Output()
	}
	
	if err != nil {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   "Failed to get cluster status",
		})
		return
	}
	
	// Parse output to find leader
	lines := strings.Split(string(output), "\n")
	var leaderNode string
	
	for _, line := range lines {
		if strings.Contains(line, "Leader") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				leaderNode = fields[1] // node name
				break
			}
		}
	}
	
	if leaderNode == "" {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   "No leader found",
		})
		return
	}
	
	// Convert node name to container name (node1 -> pg1)
	containerName := strings.Replace(leaderNode, "node", "pg", 1)
	
	// Stop the leader container
	cmd = exec.Command("docker", "stop", containerName)
	if err := cmd.Run(); err != nil {
		json.NewEncoder(w).Encode(Response{
			Success: false,
			Error:   fmt.Sprintf("Failed to stop %s: %v", containerName, err),
		})
		return
	}
	
	// Start it again after 30 seconds
	go func() {
		time.Sleep(30 * time.Second)
		cmd := exec.Command("docker", "start", containerName)
		if err := cmd.Run(); err != nil {
			log.Printf("Failed to restart %s: %v", containerName, err)
		} else {
			log.Printf("Restarted %s", containerName)
		}
	}()
	
	json.NewEncoder(w).Encode(Response{
		Success:     true,
		Message:     fmt.Sprintf("Stopped %s, will restart in 30 seconds", containerName),
		Node:        containerName,
	})
}

func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
