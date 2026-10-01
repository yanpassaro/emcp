package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"ntdsk.com/kimai/internal/kimai"
	"ntdsk.com/kimai/internal/mcpserver"
)

const DEFAULT_TIMEOUT = 30 * time.Second

func main() {
	setupLog("kimai")
	log.Println("Kimai MCP Server iniciado (stdio).")

	baseURL := strings.TrimSpace(os.Getenv("KIMAI_URL"))
	token := strings.TrimSpace(os.Getenv("KIMAI_TOKEN"))
	if baseURL == "" {
		log.Fatal("Configure a variável de ambiente KIMAI_URL (ex.: KIMAI_URL=https://kimai.exemplo.com)")
	}
	if token == "" {
		log.Fatal("Configure a variável de ambiente KIMAI_TOKEN")
	}

	client, err := kimai.NewClient(kimai.Config{
		BaseURL:    baseURL,
		Token:      token,
		HTTPClient: &http.Client{Timeout: DEFAULT_TIMEOUT},
	})
	if err != nil {
		log.Fatalf("Falha ao criar cliente Kimai: %v", err)
	}

	schedule := mcpserver.LoadSchedule(os.Getenv("KIMAI_SCHEDULE"))
	server := mcpserver.NewMCPServer(client, schedule)

	if err := server.Run(context.Background(), &mcpsdk.StdioTransport{}); err != nil {
		log.Fatalf("Kimai MCP server error: %v", err)
	}
}

func setupLog(server string) {
	baseDir := filepath.Join(userLocalDir(), "mcp", server)
	logDir := filepath.Join(baseDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("Aviso: não foi possível criar diretório de log em %s: %v", logDir, err)
		return
	}

	name := fmt.Sprintf("%s-%s.log", server, time.Now().Format("2006-01-02_15-04-05"))
	logPath := filepath.Join(logDir, name)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("Aviso: não foi possível criar log em %s: %v", logPath, err)
		return
	}

	log.SetOutput(f)
}

func userLocalDir() string {
	home := os.Getenv("USERPROFILE")
	if home != "" {
		return filepath.Join(home, ".local", "share")
	}

	home = os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, ".local", "share")
	}

	return "."
}
