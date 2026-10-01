package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"ntdsk.com/gitlab/internal/gitlab"
	"ntdsk.com/gitlab/internal/mcpserver"
)

func main() {
	setupLog("gitlab")
	log.Println("gitlab-mcp iniciado (stdio).")

	token := strings.TrimSpace(os.Getenv("GITLAB_TOKEN"))
	baseURL := os.Getenv("GITLAB_URL")

	timeout := 60
	if v := strings.TrimSpace(os.Getenv("GITLAB_HTTP_TIMEOUT_SECONDS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = n
		}
	}

	client, err := gitlab.NewClient(gitlab.Config{
		BaseURL:    baseURL,
		Token:      token,
		HTTPClient: &http.Client{Timeout: time.Duration(timeout) * time.Second},
	})
	if err != nil {
		log.Fatal(err)
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "gitlab-mcp", Version: "0.1.0"}, nil)
	mcpserver.New(client).Register(server)

	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Printf("gitlab-mcp stopped: %v", err)
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
