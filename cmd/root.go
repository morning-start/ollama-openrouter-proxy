package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
	apiKey  string
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "ollama-router",
	Short: "Ollama OpenRouter Proxy - 将 OpenRouter 免费模型暴露为 Ollama API",
	Long: `Ollama OpenRouter Proxy 是一个命令行工具，将 OpenRouter 的免费 AI 模型
通过 Ollama 兼容的 API 暴露出来，支持智能故障转移和速率限制。

主要特性:
  • 免费模型自动发现和故障转移
  • 支持 Ollama 和 OpenAI API 格式
  • 智能速率限制和失败追踪
  • 模型过滤和工具使用筛选`,
	Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "配置文件路径 (默认: $HOME/.config/ollama-router/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "启用详细日志输出")
	rootCmd.PersistentFlags().StringVarP(&apiKey, "api-key", "k", "", "OpenRouter API 密钥")

	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("openrouter.api_key", rootCmd.PersistentFlags().Lookup("api-key"))
}

func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Error:", err)
			os.Exit(1)
		}

		configDir := filepath.Join(home, ".config", "ollama-router")
		viper.AddConfigPath(configDir)
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")

		os.MkdirAll(configDir, 0755)
	}

	viper.SetEnvPrefix("OLLAMA_ROUTER")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		if verbose {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}

// getAPIKey 获取 API 密钥，优先级：命令行参数 > 环境变量 OLLAMA_ROUTER_OPENROUTER_API_KEY > 环境变量 OPENROUTER_API_KEY > 配置文件
func getAPIKey() string {
	// 1. 命令行参数（通过 viper 绑定）
	key := viper.GetString("openrouter.api_key")
	if key != "" {
		return key
	}

	// 2. 环境变量 OLLAMA_ROUTER_OPENROUTER_API_KEY
	key = os.Getenv("OLLAMA_ROUTER_OPENROUTER_API_KEY")
	if key != "" {
		return key
	}

	// 3. 环境变量 OPENROUTER_API_KEY
	key = os.Getenv("OPENROUTER_API_KEY")
	if key != "" {
		return key
	}

	return ""
}
