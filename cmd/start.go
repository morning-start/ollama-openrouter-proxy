package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"ollama-to-openrouter-proxy/internal/server"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "启动代理服务器",
	Long:  `启动 Ollama OpenRouter 代理服务器，监听指定的端口。`,
	Run:   runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)

	startCmd.Flags().StringP("port", "p", "11434", "服务器端口")
	startCmd.Flags().StringP("host", "H", "0.0.0.0", "服务器监听地址")
	startCmd.Flags().Bool("free-mode", true, "启用免费模式")
	startCmd.Flags().Bool("tool-use-only", false, "仅使用支持工具调用的模型")
	startCmd.Flags().String("log-level", "info", "日志级别 (debug, info, warn, error)")

	viper.BindPFlag("server.port", startCmd.Flags().Lookup("port"))
	viper.BindPFlag("server.host", startCmd.Flags().Lookup("host"))
	viper.BindPFlag("mode.free_mode", startCmd.Flags().Lookup("free-mode"))
	viper.BindPFlag("mode.tool_use_only", startCmd.Flags().Lookup("tool-use-only"))
	viper.BindPFlag("logging.level", startCmd.Flags().Lookup("log-level"))
}

func runStart(cmd *cobra.Command, args []string) {
	apiKey := getAPIKey()
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "错误: 未设置 OpenRouter API Key")
		fmt.Fprintln(os.Stderr, "请通过以下方式之一设置:")
		fmt.Fprintln(os.Stderr, "  1. 配置文件: openrouter.api_key")
		fmt.Fprintln(os.Stderr, "  2. 环境变量: OLLAMA_ROUTER_OPENROUTER_API_KEY 或 OPENROUTER_API_KEY")
		fmt.Fprintln(os.Stderr, "  3. 命令行参数: --api-key 或 -k")
		fmt.Fprintln(os.Stderr, "\n使用 'ollama-router config init' 进行交互式配置")
		os.Exit(1)
	}

	logLevel := viper.GetString("logging.level")
	if verbose {
		logLevel = "debug"
	}
	setupLogging(logLevel)

	port := viper.GetString("server.port")
	host := viper.GetString("server.host")
	freeMode := viper.GetBool("mode.free_mode")
	toolUseOnly := viper.GetBool("mode.tool_use_only")

	if toolUseOnly {
		os.Setenv("TOOL_USE_ONLY", "true")
	}

	configDir, _ := os.UserHomeDir()
	configDir = filepath.Join(configDir, ".config", "ollama-router")
	os.MkdirAll(configDir, 0755)

	filterPath := viper.GetString("filter.model_filter_path")
	if filterPath == "" {
		filterPath = filepath.Join(configDir, "models-filter")
	}

	srv := server.New(server.Config{
		APIKey:        apiKey,
		Host:          host,
		Port:          port,
		FreeMode:      freeMode,
		ToolUseOnly:   toolUseOnly,
		ConfigDir:     configDir,
		FilterPath:    filterPath,
		LogLevel:      logLevel,
	})

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	go func() {
		slog.Info("启动服务器", "addr", host+":"+port, "free_mode", freeMode)
		fmt.Printf("🚀 服务器已启动: http://%s:%s\n", host, port)
		fmt.Println("按 Ctrl+C 停止服务器")
		if err := srv.Start(); err != nil && err != http.ErrServerClosed {
			slog.Error("服务器启动失败", "error", err)
			os.Exit(1)
		}
	}()

	<-shutdown
	slog.Info("正在关闭服务器...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("服务器强制关闭", "error", err)
	}

	slog.Info("服务器已关闭")
}

func setupLogging(level string) {
	var slogLevel slog.Level
	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))
	slog.SetDefault(logger)
}
