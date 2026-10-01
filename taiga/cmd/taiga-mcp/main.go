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
	"ntdsk.com/taiga/internal/mcpserver"
	"ntdsk.com/taiga/internal/taiga"
)

const (
	DEFAULT_AUTH_SCHEME     = "Bearer"
	DEFAULT_TIMEOUT_SECONDS = 60
	DEFAULT_MAX_DOWNLOAD_MB = 100
	MEGABYTE                = 1024 * 1024
)

func main() {
	baseDir := setupLog("taiga")
	log.Println("=== Taiga MCP Server iniciado ===")

	config, refreshToken, username, password, err := loadConfig(baseDir)
	if err != nil {
		log.Fatal(err)
	}

	client, err := taiga.NewClient(config)
	if err != nil {
		log.Fatal(err)
	}

	if refreshToken != "" {
		refresh(client, refreshToken)
	}

	if username != "" {
		if password != "" {
			login(client, username, password)
		}
	}

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "taiga-mcp",
		Version: "0.1.0",
	}, nil)
	mcpserver.New(client).Register(server)

	log.Println("Servidor MCP pronto para receber conexões.")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Printf("Taiga MCP server stopped: %v", err)
	}
	log.Println("=== Taiga MCP Server encerrado ===")
}

func refresh(client *taiga.Client, refreshToken string) {
	log.Println("Renovando token via refresh...")
	if err := client.RefreshToken(context.Background(), refreshToken); err != nil {
		log.Printf("Aviso: refresh token falhou (%v). Tentando login.", err)
		return
	}
	log.Println("Token renovado com sucesso.")
}

func login(client *taiga.Client, username, password string) {
	log.Println("Fazendo login no Taiga...")
	if err := client.Login(context.Background(), username, password); err != nil {
		log.Printf("Aviso: login falhou (%v). Usando token estático.", err)
		return
	}
	log.Println("Login realizado com sucesso.")
}

func setupLog(server string) string {
	baseDir := filepath.Join(taiga.UserLocalDir(), "mcp", server)
	logDir := filepath.Join(baseDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("Aviso: não foi possível criar diretório de log em %s: %v", logDir, err)
		return baseDir
	}

	name := fmt.Sprintf("%s-%s.log", server, time.Now().Format("2006-01-02_15-04-05"))
	logPath := filepath.Join(logDir, name)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		log.SetOutput(os.Stderr)
		log.Printf("Aviso: não foi possível criar log em %s: %v", logPath, err)
		return baseDir
	}

	log.SetOutput(f)
	return baseDir
}

func loadConfig(defaultDownloadDir string) (taiga.Config, string, string, string, error) {
	token := strings.TrimSpace(os.Getenv("TAIGA_TOKEN"))
	refreshToken := strings.TrimSpace(os.Getenv("TAIGA_REFRESH_TOKEN"))
	username := strings.TrimSpace(os.Getenv("TAIGA_USERNAME"))
	password := strings.TrimSpace(os.Getenv("TAIGA_PASSWORD"))
	if !hasCredentials(token, refreshToken, username, password) {
		return taiga.Config{}, "", "", "", fmt.Errorf("configure TAIGA_USERNAME + TAIGA_PASSWORD, TAIGA_REFRESH_TOKEN ou TAIGA_TOKEN")
	}

	timeoutSeconds, err := envInt("TAIGA_HTTP_TIMEOUT_SECONDS", DEFAULT_TIMEOUT_SECONDS)
	if err != nil {
		return taiga.Config{}, "", "", "", fmt.Errorf("TAIGA_HTTP_TIMEOUT_SECONDS deve ser um inteiro maior que zero")
	}
	if timeoutSeconds < 1 {
		return taiga.Config{}, "", "", "", fmt.Errorf("TAIGA_HTTP_TIMEOUT_SECONDS deve ser um inteiro maior que zero")
	}

	maxDownloadMB, err := envInt("TAIGA_MAX_DOWNLOAD_MB", DEFAULT_MAX_DOWNLOAD_MB)
	if err != nil {
		return taiga.Config{}, "", "", "", fmt.Errorf("TAIGA_MAX_DOWNLOAD_MB deve ser um inteiro maior que zero")
	}
	if maxDownloadMB < 1 {
		return taiga.Config{}, "", "", "", fmt.Errorf("TAIGA_MAX_DOWNLOAD_MB deve ser um inteiro maior que zero")
	}

	downloadDir := strings.TrimSpace(os.Getenv("TAIGA_DOWNLOAD_DIR"))
	if downloadDir == "" {
		downloadDir = filepath.Join(defaultDownloadDir, "downloads")
	}

	return taiga.Config{
		BaseURL:          os.Getenv("TAIGA_URL"),
		Token:            token,
		AuthScheme:       getenvDefault("TAIGA_AUTH_SCHEME", DEFAULT_AUTH_SCHEME),
		HTTPClient:       &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second},
		DownloadDir:      downloadDir,
		MaxDownloadBytes: int64(maxDownloadMB) * MEGABYTE,
	}, refreshToken, username, password, nil
}

func hasCredentials(token, refreshToken, username, password string) bool {
	if token != "" {
		return true
	}
	if refreshToken != "" {
		return true
	}
	if username == "" {
		return false
	}
	return password != ""
}

func envInt(name string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback, nil
	}
	return strconv.Atoi(value)
}

func getenvDefault(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value != "" {
		return value
	}
	return fallback
}
