package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/fatih/color"
	_ "github.com/lib/pq"
)

type Stats struct {
	TotalWrites      int64
	TotalReads       int64
	SuccessfulWrites int64
	SuccessfulReads  int64
	FailedWrites     int64
	FailedReads      int64
	Reconnects       int64
	CurrentNode      string
	StartTime        time.Time
}

type App struct {
	db    *sql.DB
	stats Stats
}

func main() {
	// Colors
	green := color.New(color.FgGreen, color.Bold)
	red := color.New(color.FgRed, color.Bold)
	yellow := color.New(color.FgYellow, color.Bold)
	cyan := color.New(color.FgCyan, color.Bold)

	cyan.Println("╔══════════════════════════════════════════════════════════════╗")
	cyan.Println("║                                                              ║")
	cyan.Println("║        PostgreSQL HA Test Application                       ║")
	cyan.Println("║        Testing Automatic Failover                           ║")
	cyan.Println("║                                                              ║")
	cyan.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Database connection string (via HAProxy)
	connStr := "host=localhost port=54320 user=postgres password=admin_password dbname=postgres sslmode=disable"
	
	app := &App{
		stats: Stats{
			StartTime: time.Now(),
		},
	}

	// Initial connection
	yellow.Println("🔌 Connecting to PostgreSQL via HAProxy...")
	if err := app.connect(connStr); err != nil {
		red.Printf("❌ Failed to connect: %v\n", err)
		os.Exit(1)
	}
	green.Println("✅ Connected successfully!")
	fmt.Println()

	// Initialize database
	if err := app.initDB(); err != nil {
		red.Printf("❌ Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	green.Println("✅ Database initialized!")
	fmt.Println()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Start monitoring goroutine
	go app.monitor()

	// Start write/read loop
	go app.workload()

	// Wait for interrupt
	<-sigChan
	fmt.Println()
	yellow.Println("\n🛑 Shutting down...")
	app.printFinalStats()
	app.db.Close()
}

func (app *App) connect(connStr string) error {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)

	// Test connection
	if err := db.Ping(); err != nil {
		return err
	}

	app.db = db
	app.stats.Reconnects++
	
	// Get current node info
	app.getCurrentNode()
	
	return nil
}

func (app *App) initDB() error {
	// Create table if not exists
	_, err := app.db.Exec(`
		CREATE TABLE IF NOT EXISTS ha_test (
			id SERIAL PRIMARY KEY,
			message TEXT NOT NULL,
			node_name TEXT,
			created_at TIMESTAMP DEFAULT NOW()
		)
	`)
	return err
}

func (app *App) getCurrentNode() {
	var nodeName string
	err := app.db.QueryRow("SELECT inet_server_addr()").Scan(&nodeName)
	if err == nil {
		app.stats.CurrentNode = nodeName
	}
}

func (app *App) workload() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Write operation
		app.stats.TotalWrites++
		if err := app.writeData(); err != nil {
			app.stats.FailedWrites++
			color.Red("❌ Write failed: %v", err)
			
			// Try to reconnect
			app.tryReconnect()
		} else {
			app.stats.SuccessfulWrites++
		}

		// Read operation
		app.stats.TotalReads++
		if err := app.readData(); err != nil {
			app.stats.FailedReads++
			color.Red("❌ Read failed: %v", err)
			
			// Try to reconnect
			app.tryReconnect()
		} else {
			app.stats.SuccessfulReads++
		}
	}
}

func (app *App) writeData() error {
	message := fmt.Sprintf("Test message at %s", time.Now().Format("15:04:05"))
	
	var nodeName string
	err := app.db.QueryRow("SELECT inet_server_addr()").Scan(&nodeName)
	if err != nil {
		return err
	}

	_, err = app.db.Exec(
		"INSERT INTO ha_test (message, node_name) VALUES ($1, $2)",
		message, nodeName,
	)
	
	if err == nil {
		// Check if node changed (failover detected)
		if app.stats.CurrentNode != "" && app.stats.CurrentNode != nodeName {
			color.Yellow("\n🔄 FAILOVER DETECTED!")
			color.Yellow("   Old node: %s", app.stats.CurrentNode)
			color.Yellow("   New node: %s", nodeName)
			fmt.Println()
		}
		app.stats.CurrentNode = nodeName
	}
	
	return err
}

func (app *App) readData() error {
	var count int
	err := app.db.QueryRow("SELECT COUNT(*) FROM ha_test").Scan(&count)
	return err
}

func (app *App) tryReconnect() {
	color.Yellow("🔄 Attempting to reconnect...")
	
	connStr := "host=localhost port=54320 user=postgres password=admin_password dbname=postgres sslmode=disable"
	
	maxRetries := 5
	for i := 0; i < maxRetries; i++ {
		time.Sleep(2 * time.Second)
		
		if err := app.connect(connStr); err != nil {
			color.Red("   Retry %d/%d failed: %v", i+1, maxRetries, err)
		} else {
			color.Green("✅ Reconnected successfully!")
			app.getCurrentNode()
			return
		}
	}
	
	color.Red("❌ Failed to reconnect after %d attempts", maxRetries)
}

func (app *App) monitor() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		app.printStats()
	}
}

func (app *App) printStats() {
	// Clear screen (optional)
	// fmt.Print("\033[H\033[2J")
	
	uptime := time.Since(app.stats.StartTime)
	
	cyan := color.New(color.FgCyan, color.Bold)
	green := color.New(color.FgGreen)
	red := color.New(color.FgRed)
	
	fmt.Println()
	cyan.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	cyan.Println("                    STATISTICS")
	cyan.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	
	fmt.Printf("⏱️  Uptime:           %s\n", uptime.Round(time.Second))
	fmt.Printf("🖥️  Current Node:     %s\n", app.stats.CurrentNode)
	fmt.Printf("🔄 Reconnects:       %d\n", app.stats.Reconnects)
	fmt.Println()
	
	fmt.Printf("📝 Total Writes:     %d\n", app.stats.TotalWrites)
	green.Printf("   ✓ Successful:     %d (%.1f%%)\n", 
		app.stats.SuccessfulWrites,
		float64(app.stats.SuccessfulWrites)/float64(max(app.stats.TotalWrites, 1))*100)
	red.Printf("   ✗ Failed:         %d (%.1f%%)\n",
		app.stats.FailedWrites,
		float64(app.stats.FailedWrites)/float64(max(app.stats.TotalWrites, 1))*100)
	fmt.Println()
	
	fmt.Printf("📖 Total Reads:      %d\n", app.stats.TotalReads)
	green.Printf("   ✓ Successful:     %d (%.1f%%)\n",
		app.stats.SuccessfulReads,
		float64(app.stats.SuccessfulReads)/float64(max(app.stats.TotalReads, 1))*100)
	red.Printf("   ✗ Failed:         %d (%.1f%%)\n",
		app.stats.FailedReads,
		float64(app.stats.FailedReads)/float64(max(app.stats.TotalReads, 1))*100)
	
	cyan.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
}

func (app *App) printFinalStats() {
	cyan := color.New(color.FgCyan, color.Bold)
	green := color.New(color.FgGreen, color.Bold)
	
	uptime := time.Since(app.stats.StartTime)
	
	fmt.Println()
	cyan.Println("╔══════════════════════════════════════════════════════════════╗")
	cyan.Println("║                    FINAL STATISTICS                          ║")
	cyan.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	
	fmt.Printf("⏱️  Total Uptime:     %s\n", uptime.Round(time.Second))
	fmt.Printf("🔄 Total Reconnects: %d\n", app.stats.Reconnects)
	fmt.Println()
	
	fmt.Printf("📝 Write Operations:\n")
	fmt.Printf("   Total:            %d\n", app.stats.TotalWrites)
	green.Printf("   Successful:       %d (%.1f%%)\n",
		app.stats.SuccessfulWrites,
		float64(app.stats.SuccessfulWrites)/float64(max(app.stats.TotalWrites, 1))*100)
	fmt.Printf("   Failed:           %d (%.1f%%)\n",
		app.stats.FailedWrites,
		float64(app.stats.FailedWrites)/float64(max(app.stats.TotalWrites, 1))*100)
	fmt.Println()
	
	fmt.Printf("📖 Read Operations:\n")
	fmt.Printf("   Total:            %d\n", app.stats.TotalReads)
	green.Printf("   Successful:       %d (%.1f%%)\n",
		app.stats.SuccessfulReads,
		float64(app.stats.SuccessfulReads)/float64(max(app.stats.TotalReads, 1))*100)
	fmt.Printf("   Failed:           %d (%.1f%%)\n",
		app.stats.FailedReads,
		float64(app.stats.FailedReads)/float64(max(app.stats.TotalReads, 1))*100)
	fmt.Println()
	
	// Calculate availability
	totalOps := app.stats.TotalWrites + app.stats.TotalReads
	successfulOps := app.stats.SuccessfulWrites + app.stats.SuccessfulReads
	availability := float64(successfulOps) / float64(max(totalOps, 1)) * 100
	
	if availability >= 99.9 {
		green.Printf("🎯 Availability:     %.2f%% (Excellent!)\n", availability)
	} else if availability >= 99.0 {
		color.Yellow("🎯 Availability:     %.2f%% (Good)\n", availability)
	} else {
		color.Red("🎯 Availability:     %.2f%% (Needs improvement)\n", availability)
	}
	
	fmt.Println()
}

func max(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
