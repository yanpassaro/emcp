package main

import (
	"context"
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

func main() {
	setupLog("kimai")
	log.Println("Kimai MCP Server iniciado (stdio).")

	baseURL := strings.TrimSpace(os.Getenv("KIMAI_URL"))
	token := strings.TrimSpace(os.Getenv("KIMAI_TOKEN"))
	if baseURL == "" || token == "" {
		log.Fatal("Configure as variáveis de ambiente KIMAI_URL e KIMAI_TOKEN (ex.: KIMAI_URL=https://kimai.exemplo.com KIMAI_TOKEN=seu-token)")
	}

	client, err := kimai.NewClient(kimai.Config{
		BaseURL:    baseURL,
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
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
	os.MkdirAll(logDir, 0o755)
	logPath := filepath.Join(logDir, server+"-"+time.Now().Format("2006-01-02_15-04-05")+".log")
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); err == nil {
		log.SetOutput(f)
	} else {
		log.SetOutput(os.Stderr)
		log.Printf("Aviso: não foi possível criar log em %s: %v", logPath, err)
	}
}

func userLocalDir() string {
	if home := os.Getenv("USERPROFILE"); home != "" {
		return filepath.Join(home, ".local", "share")
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".local", "share")
	}
	return "."
}
