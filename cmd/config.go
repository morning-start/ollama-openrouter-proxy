package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "配置管理",
	Long:  `管理 ollama-router 的配置文件和设置。`,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "交互式初始化配置",
	Long:  `通过交互式向导创建初始配置文件。`,
	Run:   runConfigInit,
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "显示当前配置",
	Long:  `显示当前加载的配置文件内容。`,
	Run:   runConfigShow,
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "设置配置项",
	Long:  `设置指定的配置项并保存到配置文件。`,
	Args:  cobra.ExactArgs(2),
	Run:   runConfigSet,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "获取配置项",
	Long:  `获取指定配置项的值。`,
	Args:  cobra.ExactArgs(1),
	Run:   runConfigGet,
}

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configInitCmd)
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configGetCmd)
}

func runConfigInit(cmd *cobra.Command, args []string) {
	reader := bufio.NewReader(os.Stdin)

	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	fmt.Println(cyan("🚀 Ollama Router 配置向导"))
	fmt.Println("========================")
	fmt.Println()

	config := make(map[string]interface{})

	fmt.Print("请输入 OpenRouter API Key: ")
	apiKey, _ := reader.ReadString('\n')
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "错误: API Key 不能为空")
		os.Exit(1)
	}
	config["openrouter.api_key"] = apiKey

	fmt.Println()
	fmt.Println(yellow("服务器配置:"))

	fmt.Print("监听端口 [11434]: ")
	port, _ := reader.ReadString('\n')
	port = strings.TrimSpace(port)
	if port == "" {
		port = "11434"
	}
	config["server.port"] = port

	fmt.Print("监听地址 [0.0.0.0]: ")
	host, _ := reader.ReadString('\n')
	host = strings.TrimSpace(host)
	if host == "" {
		host = "0.0.0.0"
	}
	config["server.host"] = host

	fmt.Println()
	fmt.Println(yellow("运行模式:"))

	fmt.Print("启用免费模式? [Y/n]: ")
	freeMode, _ := reader.ReadString('\n')
	freeMode = strings.TrimSpace(strings.ToLower(freeMode))
	config["mode.free_mode"] = freeMode != "n" && freeMode != "no"

	fmt.Print("仅使用支持工具调用的模型? [y/N]: ")
	toolUse, _ := reader.ReadString('\n')
	toolUse = strings.TrimSpace(strings.ToLower(toolUse))
	config["mode.tool_use_only"] = toolUse == "y" || toolUse == "yes"

	fmt.Println()
	fmt.Println(yellow("日志配置:"))

	fmt.Print("日志级别 [info]: ")
	logLevel, _ := reader.ReadString('\n')
	logLevel = strings.TrimSpace(logLevel)
	if logLevel == "" {
		logLevel = "info"
	}
	config["logging.level"] = logLevel

	home, _ := os.UserHomeDir()
	configDir := filepath.Join(home, ".config", "ollama-router")
	configFile := filepath.Join(configDir, "config.yaml")

	os.MkdirAll(configDir, 0755)

	for key, value := range config {
		viper.Set(key, value)
	}

	if err := viper.WriteConfigAs(configFile); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存配置失败: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println(green("✅ 配置已保存到:"), configFile)
	fmt.Println()
	fmt.Println("你可以使用以下命令启动服务器:")
	fmt.Println(green("  ollama-router start"))
	fmt.Println()
	fmt.Println("或使用自定义配置:")
	fmt.Println(green("  ollama-router -c " + configFile + " start"))
}

func runConfigShow(cmd *cobra.Command, args []string) {
	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()

	fmt.Println(cyan("当前配置:"))
	fmt.Println("==========")
	fmt.Println()

	settings := []struct {
		key   string
		title string
	}{
		{"openrouter.api_key", "OpenRouter API Key"},
		{"server.port", "服务器端口"},
		{"server.host", "服务器地址"},
		{"mode.free_mode", "免费模式"},
		{"mode.tool_use_only", "仅工具模型"},
		{"logging.level", "日志级别"},
	}

	for _, s := range settings {
		value := viper.Get(s.key)
		if s.key == "openrouter.api_key" && value != "" {
			value = maskAPIKey(value.(string))
		}
		fmt.Printf("%s: %v\n", yellow(s.title), value)
	}

	if viper.ConfigFileUsed() != "" {
		fmt.Println()
		fmt.Println("配置文件:", viper.ConfigFileUsed())
	} else {
		fmt.Println()
		fmt.Println(yellow("注意: 未找到配置文件，使用默认设置"))
	}
}

func runConfigSet(cmd *cobra.Command, args []string) {
	key := args[0]
	value := args[1]

	var typedValue interface{}
	typedValue = value

	if boolVal, err := strconv.ParseBool(value); err == nil {
		typedValue = boolVal
	} else if intVal, err := strconv.Atoi(value); err == nil {
		typedValue = intVal
	}

	viper.Set(key, typedValue)

	configFile := viper.ConfigFileUsed()
	if configFile == "" {
		home, _ := os.UserHomeDir()
		configDir := filepath.Join(home, ".config", "ollama-router")
		os.MkdirAll(configDir, 0755)
		configFile = filepath.Join(configDir, "config.yaml")
	}

	if err := viper.WriteConfigAs(configFile); err != nil {
		fmt.Fprintf(os.Stderr, "错误: 保存配置失败: %v\n", err)
		os.Exit(1)
	}

	green := color.New(color.FgGreen).SprintFunc()
	fmt.Printf("%s 已设置为: %v\n", green(key), typedValue)
	fmt.Println("配置已保存到:", configFile)
}

func runConfigGet(cmd *cobra.Command, args []string) {
	key := args[0]
	value := viper.Get(key)

	if value == nil {
		fmt.Fprintf(os.Stderr, "配置项 '%s' 不存在\n", key)
		os.Exit(1)
	}

	if key == "openrouter.api_key" && value != "" {
		value = maskAPIKey(value.(string))
	}

	fmt.Println(value)
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}
