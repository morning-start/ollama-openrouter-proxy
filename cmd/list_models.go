package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var listModelsCmd = &cobra.Command{
	Use:   "list-models",
	Short: "列出可用的免费模型",
	Long:  `从 OpenRouter 获取并显示所有可用的免费模型列表。`,
	Run:   runListModels,
}

func init() {
	rootCmd.AddCommand(listModelsCmd)

	listModelsCmd.Flags().Bool("tool-use-only", false, "仅显示支持工具调用的模型")
	listModelsCmd.Flags().Bool("json", false, "以 JSON 格式输出")
	listModelsCmd.Flags().String("filter", "", "过滤模型名称（支持部分匹配）")
}

type modelDetail struct {
	ID            string `json:"id"`
	ContextLength int    `json:"context_length"`
	SupportsTools bool   `json:"supports_tools"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

type orModelsResponse struct {
	Data []struct {
		ID                  string   `json:"id"`
		ContextLength       int      `json:"context_length"`
		SupportedParameters []string `json:"supported_parameters"`
		TopProvider         struct {
			ContextLength int `json:"context_length"`
		} `json:"top_provider"`
		Pricing struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		} `json:"pricing"`
	} `json:"data"`
}

func runListModels(cmd *cobra.Command, args []string) {
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

	toolUseOnly, _ := cmd.Flags().GetBool("tool-use-only")
	jsonOutput, _ := cmd.Flags().GetBool("json")
	filterPattern, _ := cmd.Flags().GetString("filter")

	fmt.Println("⏳ 正在获取免费模型列表...")

	models, err := fetchFreeModelsWithDetails(apiKey, toolUseOnly)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: 获取模型失败: %v\n", err)
		os.Exit(1)
	}

	if filterPattern != "" {
		filtered := make([]modelDetail, 0)
		for _, m := range models {
			if strings.Contains(strings.ToLower(m.ID), strings.ToLower(filterPattern)) {
				filtered = append(filtered, m)
			}
		}
		models = filtered
	}

	if jsonOutput {
		outputJSON(models)
	} else {
		outputTable(models)
	}
}

func fetchFreeModelsWithDetails(apiKey string, toolUseOnly bool) ([]modelDetail, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var result orModelsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []modelDetail
	for _, m := range result.Data {
		if m.Pricing.Prompt != "0" || m.Pricing.Completion != "0" {
			continue
		}

		supportsTools := supportsToolUseCheck(m.SupportedParameters)
		if toolUseOnly && !supportsTools {
			continue
		}

		ctx := m.TopProvider.ContextLength
		if ctx == 0 {
			ctx = m.ContextLength
		}

		models = append(models, modelDetail{
			ID:            m.ID,
			ContextLength: ctx,
			SupportsTools: supportsTools,
			Pricing: struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			}{
				Prompt:     m.Pricing.Prompt,
				Completion: m.Pricing.Completion,
			},
		})
	}

	return models, nil
}

func supportsToolUseCheck(supportedParams []string) bool {
	for _, param := range supportedParams {
		if param == "tools" || param == "tool_choice" {
			return true
		}
	}
	return false
}

func outputJSON(models []modelDetail) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	encoder.Encode(models)
}

func outputTable(models []modelDetail) {
	if len(models) == 0 {
		fmt.Println("⚠️  没有找到符合条件的免费模型")
		return
	}

	fmt.Printf("\n✅ 找到 %d 个免费模型\n\n", len(models))

	green := color.New(color.FgGreen).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	cyan := color.New(color.FgCyan).SprintFunc()

	fmt.Printf("%-40s %12s %12s %10s\n", "模型名称", "上下文长度", "工具支持", "价格")
	fmt.Println(strings.Repeat("-", 80))

	for _, m := range models {
		toolSupport := "❌"
		if m.SupportsTools {
			toolSupport = green("✓")
		}

		contextLen := formatContextLength(m.ContextLength)

		parts := strings.Split(m.ID, "/")
		displayName := parts[len(parts)-1]

		fmt.Printf("%-40s %12s %12s %10s\n",
			cyan(displayName),
			yellow(contextLen),
			toolSupport,
			green("免费"),
		)
	}

	fmt.Println()
	fmt.Println("💡 提示:")
	fmt.Println("  • 使用 --tool-use-only 只显示支持工具调用的模型")
	fmt.Println("  • 使用 --filter <关键词> 过滤模型名称")
	fmt.Println("  • 使用 --json 以 JSON 格式输出")

	configDir, _ := os.UserHomeDir()
	configDir = filepath.Join(configDir, ".config", "ollama-router")
	fmt.Printf("\n📁 配置目录: %s\n", configDir)
}

func formatContextLength(length int) string {
	if length >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(length)/1000000)
	}
	if length >= 1000 {
		return fmt.Sprintf("%.1fK", float64(length)/1000)
	}
	return fmt.Sprintf("%d", length)
}
