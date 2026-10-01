package main

import (
	"context"
	"fmt"
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

const DEFAULT_TIMEOUT_SECONDS = 60

func main() {
	setupLog("gitlab")
	log.Println("gitlab-mcp iniciado (stdio).")

	token := strings.TrimSpace(os.Getenv("GITLAB_TOKEN"))
	baseURL := os.Getenv("GITLAB_URL")

	timeout := DEFAULT_TIMEOUT_SECONDS
	v := strings.TrimSpace(os.Getenv("GITLAB_HTTP_TIMEOUT_SECONDS"))
	if v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			if n > 0 {
				timeout = n
			}
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
