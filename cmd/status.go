package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "检查服务状态",
	Long:  `检查代理服务器运行状态和模型可用性。`,
	Run:   runStatus,
}

func init() {
	rootCmd.AddCommand(statusCmd)

	statusCmd.Flags().StringP("host", "H", "localhost", "服务器主机")
	statusCmd.Flags().StringP("port", "p", "11434", "服务器端口")
}

func runStatus(cmd *cobra.Command, args []string) {
	host, _ := cmd.Flags().GetString("host")
	port, _ := cmd.Flags().GetString("port")

	cyan := color.New(color.FgCyan).SprintFunc()
	yellow := color.New(color.FgYellow).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()
	red := color.New(color.FgRed).SprintFunc()

	fmt.Println(cyan("📊 服务状态检查"))
	fmt.Println("==============")
	fmt.Println()

	baseURL := fmt.Sprintf("http://%s:%s", host, port)

	fmt.Println("检查服务器健康状态...")
	if err := checkHealth(baseURL); err != nil {
		fmt.Printf("%s 服务器未运行: %v\n", red("✗"), err)
		fmt.Println()
		fmt.Println("使用以下命令启动服务器:")
		fmt.Println(green("  ollama-router start"))
		return
	}
	fmt.Printf("%s 服务器运行正常\n", green("✓"))
	fmt.Println()

	fmt.Println("获取可用模型列表...")
	models, err := getModels(baseURL)
	if err != nil {
		fmt.Printf("%s 获取模型列表失败: %v\n", red("✗"), err)
		return
	}
	fmt.Printf("%s 找到 %d 个可用模型\n", green("✓"), len(models))
	fmt.Println()

	if len(models) > 0 {
		fmt.Println("可用模型:")
		fmt.Println()
		for _, model := range models {
			if name, ok := model["name"].(string); ok {
				fmt.Printf("  • %s\n", cyan(name))
			}
		}
	}

	fmt.Println()
	fmt.Println("配置信息:")
	fmt.Printf("  服务器地址: %s\n", yellow(baseURL))
	fmt.Printf("  免费模式: %s\n", green(viper.GetBool("mode.free_mode")))
	fmt.Printf("  工具模型: %s\n", green(viper.GetBool("mode.tool_use_only")))
}

func checkHealth(baseURL string) error {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Get(baseURL + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %s", resp.Status)
	}

	return nil
}

func getModels(baseURL string) ([]map[string]interface{}, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := client.Get(baseURL + "/api/tags")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %s", resp.Status)
	}

	var result struct {
		Models []map[string]interface{} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Models, nil
}
