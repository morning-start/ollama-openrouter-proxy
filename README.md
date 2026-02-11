# Ollama OpenRouter 代理

一个代理服务器，通过 Ollama 兼容的 API 将 OpenRouter 的免费 AI 模型暴露出来，具有智能重试逻辑和速率限制保护。我主要用它来测试。

## 功能特性

- **免费模式（默认）**：自动从 OpenRouter 选择并使用免费模型，具有智能故障转移功能。除非设置 `FREE_MODE=false`，否则默认启用。
- **模型过滤**：创建一个 `models-filter/filter` 文件，每行一个模型名称模式。支持部分匹配 - `gemini` 可以匹配 `gemini-2.0-flash-exp:free`。在免费和非免费模式下都有效。
- **工具调用过滤**：通过设置 `TOOL_USE_ONLY=true` 仅过滤支持函数调用/工具使用的免费模型。根据模型的 `supported_parameters` 是否包含 "tools" 或 "tool_choice" 进行过滤。
- **Ollama 风格 API**：服务器监听 `11434` 端口，暴露类似 Ollama 的端点（如 `/api/chat`、`/api/tags`）。
- **模型列表**：从 OpenRouter 获取可用模型列表。
- **模型详情**：检索特定模型的元数据。
- **流式聊天**：以与 Ollama 兼容的分块 JSON 格式转发来自 OpenRouter 的流式响应。
- **命令行界面**：易于使用的 CLI，支持配置管理、模型列表和缓存控制。

## 安装

### 从源码编译

```bash
git clone https://github.com/your-username/ollama-openrouter-proxy.git
cd ollama-openrouter-proxy
go build -o ollama-router .
```

### 使用 Go Install

```bash
go install github.com/your-username/ollama-openrouter-proxy@latest
```

## 快速开始

### 1. 初始化配置

```bash
ollama-router config init
```

这个交互式向导将引导你完成 API 密钥和偏好设置的配置。

### 2. 启动服务器

```bash
ollama-router start
```

### 3. 列出可用模型

```bash
ollama-router list-models
```

## CLI 使用

### 全局选项

```bash
ollama-router [全局选项] <命令> [命令选项]

全局选项:
  -c, --config string   配置文件路径 (默认: $HOME/.config/ollama-router/config.yaml)
  -k, --api-key string  OpenRouter API 密钥
  -v, --verbose         启用详细日志
  --version             显示版本信息
```

### API 密钥配置方式（优先级从高到低）

1. **命令行参数**（最高优先级）

   ```bash
   ollama-router -k your-api-key start
   # 或
   ollama-router --api-key your-api-key start
   ```

2. **环境变量**

   ```bash
   export OLLAMA_ROUTER_OPENROUTER_API_KEY=your-api-key
   ollama-router start
   ```

3. **环境变量（备用）**

   ```bash
   export OPENROUTER_API_KEY=your-api-key
   ollama-router start
   ```

4. **配置文件**（最低优先级）
   ```yaml
   openrouter:
     api_key: "your-api-key"
   ```

### 命令

#### `start` - 启动代理服务器

```bash
# 使用默认设置启动
ollama-router start

# 在自定义端口启动
ollama-router start -p 8080

# 不使用免费模式启动
ollama-router start --free-mode=false

# 仅使用支持工具调用的模型启动
ollama-router start --tool-use-only
```

选项:

- `-p, --port`: 服务器端口 (默认: 11434)
- `-H, --host`: 服务器主机 (默认: 0.0.0.0)
- `--free-mode`: 启用免费模式 (默认: true)
- `--tool-use-only`: 仅使用支持工具使用的模型 (默认: false)
- `--api-key`: OpenRouter API 密钥
- `--log-level`: 日志级别 - debug, info, warn, error (默认: info)

#### `list-models` - 列出可用的免费模型

```bash
# 列出所有免费模型
ollama-router list-models

# 仅列出支持工具调用的模型
ollama-router list-models --tool-use-only

# 按名称过滤
ollama-router list-models --filter gemini

# 以 JSON 格式输出
ollama-router list-models --json
```

选项:

- `--tool-use-only`: 仅显示支持工具调用的模型
- `--json`: 以 JSON 格式输出
- `--filter`: 按名称模式过滤模型

#### `config` - 配置管理

```bash
# 交互式配置设置
ollama-router config init

# 显示当前配置
ollama-router config show

# 设置配置值
ollama-router config set server.port 8080
ollama-router config set openrouter.api_key your-api-key

# 获取配置值
ollama-router config get server.port
```

#### `cache` - 缓存管理

```bash
# 查看缓存状态
ollama-router cache status

# 刷新模型缓存
ollama-router cache refresh

# 清除所有缓存
ollama-router cache clear
```

#### `status` - 检查服务器状态

```bash
# 检查本地服务器状态
ollama-router status

# 检查远程服务器状态
ollama-router status -H remote-host -p 11434
```

### 配置文件

配置以 YAML 格式存储：

**位置：**

- Windows: `%USERPROFILE%\.config\ollama-router\config.yaml`
- Linux/macOS: `~/.config/ollama-router/config.yaml`

**配置示例：**

```yaml
openrouter:
  api_key: "your-api-key"

server:
  port: "11434"
  host: "0.0.0.0"

mode:
  free_mode: true
  tool_use_only: false

logging:
  level: "info"
```

## 环境变量

你也可以使用环境变量配置代理：

| 变量                               | 描述                                | 默认值  |
| ---------------------------------- | ----------------------------------- | ------- |
| `OPENROUTER_API_KEY`               | 你的 OpenRouter API 密钥（必需）    | -       |
| `OLLAMA_ROUTER_OPENROUTER_API_KEY` | 备用 API 密钥变量                   | -       |
| `FREE_MODE`                        | 仅使用免费模型                      | `true`  |
| `TOOL_USE_ONLY`                    | 仅过滤支持函数调用的模型            | `false` |
| `LOG_LEVEL`                        | 日志级别 (DEBUG, INFO, WARN, ERROR) | `INFO`  |
| `PORT`                             | 服务器端口                          | `11434` |
| `FAILURE_COOLDOWN_MINUTES`         | 临时失败的冷却时间                  | `5`     |
| `RATELIMIT_COOLDOWN_MINUTES`       | 速率限制错误的冷却时间              | `1`     |
| `CACHE_TTL_HOURS`                  | 模型缓存 TTL                        | `24`    |

## API 端点

代理提供 Ollama 兼容和 OpenAI 兼容的端点：

### Ollama API 端点

| 方法     | 端点              | 描述                                |
| -------- | ----------------- | ----------------------------------- |
| `GET`    | `/`               | 健康检查 - 返回 "Ollama is running" |
| `HEAD`   | `/`               | 健康检查（HEAD 请求）               |
| `GET`    | `/api/version`    | 获取版本信息                        |
| `POST`   | `/api/generate`   | 生成文本完成（支持流式）            |
| `POST`   | `/api/chat`       | 聊天完成（支持流式）                |
| `GET`    | `/api/tags`       | 列出本地可用模型                    |
| `POST`   | `/api/show`       | 显示模型信息                        |
| `POST`   | `/api/create`     | 创建模型（OpenRouter 不支持）       |
| `POST`   | `/api/copy`       | 复制模型（OpenRouter 不支持）       |
| `DELETE` | `/api/delete`     | 删除模型（OpenRouter 不支持）       |
| `POST`   | `/api/pull`       | 拉取模型（OpenRouter 不需要）       |
| `POST`   | `/api/push`       | 推送模型（OpenRouter 不支持）       |
| `POST`   | `/api/embeddings` | 生成文本嵌入向量                    |
| `GET`    | `/api/ps`         | 列出运行中的模型                    |

#### 示例请求

**列出模型：**

```bash
curl http://localhost:11434/api/tags
```

**生成文本完成：**

```bash
curl -X POST http://localhost:11434/api/generate \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat-v3-0324:free",
    "prompt": "天空为什么是蓝色的？",
    "stream": false
  }'
```

**聊天完成：**

```bash
curl -X POST http://localhost:11434/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat-v3-0324:free",
    "messages": [
      {"role": "user", "content": "你好！"}
    ],
    "stream": true
  }'
```

**模型详情：**

```bash
curl -X POST http://localhost:11434/api/show \
  -H "Content-Type: application/json" \
  -d '{"name": "deepseek-chat-v3-0324:free"}'
```

**生成嵌入向量：**

```bash
curl -X POST http://localhost:11434/api/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat-v3-0324:free",
    "prompt": "Hello world"
  }'
```

**查看运行中的模型：**

```bash
curl http://localhost:11434/api/ps
```

**获取版本信息：**

```bash
curl http://localhost:11434/api/version
```

### OpenAI API 端点

| 方法   | 端点                   | 描述                       |
| ------ | ---------------------- | -------------------------- |
| `GET`  | `/v1/models`           | 以 OpenAI 格式列出可用模型 |
| `POST` | `/v1/chat/completions` | 支持流式的聊天完成         |
| `POST` | `/v1/embeddings`       | 生成文本嵌入向量           |

#### 示例请求

**列出模型（OpenAI 格式）：**

```bash
curl http://localhost:11434/v1/models
```

**聊天完成（OpenAI 格式）：**

```bash
curl -X POST http://localhost:11434/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat-v3-0324:free",
    "messages": [
      {"role": "user", "content": "你好！"}
    ],
    "stream": false
  }'
```

**生成嵌入向量（OpenAI 格式）：**

```bash
curl -X POST http://localhost:11434/v1/embeddings \
  -H "Content-Type: application/json" \
  -d '{
    "model": "deepseek-chat-v3-0324:free",
    "input": "Hello world"
  }'
```

## 免费模式（默认行为）

代理默认在**免费模式**下运行，自动从 OpenRouter 的可用免费模型中选择。这提供了无需手动选择模型的经济高效使用方式。

```bash
# 默认启用免费模式 - 无需配置
ollama-router start

# 要禁用免费模式并使用所有可用模型
ollama-router start --free-mode=false

# 要仅使用支持工具调用/函数调用的免费模型
ollama-router start --tool-use-only
```

### 免费模式工作原理

- **自动模型发现**：从 OpenRouter 获取并缓存可用的免费模型
- **智能故障转移**：如果请求的模型失败，自动尝试其他可用的免费模型
- **失败追踪**：临时跳过最近失败的模型（可配置冷却时间）
- **模型优先级**：按上下文长度顺序尝试模型（最大的优先）
- **缓存管理**：维护 `free-models` 文件以实现快速启动，以及 `failures.db` SQLite 数据库用于失败追踪

启动后，代理监听 `11434` 端口。你可以使用与 Ollama 兼容的工具向 `http://localhost:11434` 发送请求。

## Docker 使用

### 使用 Docker Compose

1. **克隆仓库并创建环境文件：**

   ```bash
   git clone https://github.com/your-username/ollama-openrouter-proxy.git
   cd ollama-openrouter-proxy
   cp .env.example .env
   ```

2. **使用你的 OpenRouter API 密钥编辑 `.env` 文件：**

   ```bash
   OPENROUTER_API_KEY=your-openrouter-api-key
   FREE_MODE=true
   TOOL_USE_ONLY=false
   ```

3. **可选：创建模型过滤器：**

   ```bash
   mkdir -p models-filter
   echo "gemini" > models-filter/filter  # 仅显示 Gemini 模型
   ```

4. **可选：启用工具调用过滤：**
   在 `.env` 文件中设置 `TOOL_USE_ONLY=true` 以仅使用支持函数调用/工具使用的模型。

5. **使用 Docker Compose 运行：**
   ```bash
   docker compose up -d
   ```

服务将在 `http://localhost:11434` 上可用。

### 直接使用 Docker

```bash
docker build -t ollama-proxy .
docker run -p 11434:11434 -e OPENROUTER_API_KEY="your-openrouter-api-key" ollama-proxy

# 要启用工具调用过滤
docker run -p 11434:11434 -e OPENROUTER_API_KEY="your-openrouter-api-key" -e TOOL_USE_ONLY=true ollama-proxy
```

## 模型过滤

在配置目录中创建 `models-filter/filter` 文件来过滤可用模型：

```bash
# 创建过滤目录和文件
mkdir -p ~/.config/ollama-router/models-filter
echo "gemini" > ~/.config/ollama-router/models-filter/filter
echo "deepseek" >> ~/.config/ollama-router/models-filter/filter
```

这将仅显示名称中包含 "gemini" 或 "deepseek" 的模型。

## 故障排查

### 服务器无法启动

```bash
# 检查配置
ollama-router config show

# 验证 API 密钥已设置
ollama-router config get openrouter.api_key
```

### 没有可用模型

```bash
# 刷新模型缓存
ollama-router cache refresh

# 检查缓存状态
ollama-router cache status
```

### 连接问题

```bash
# 检查服务器状态
ollama-router status

# 使用详细日志测试
ollama-router -v start
```

## 致谢

本项目灵感来源于 [xsharov/enchanted-ollama-openrouter-proxy](https://github.com/xsharov/enchanted-ollama-openrouter-proxy)，其灵感来源于 [marknefedov](https://github.com/marknefedov/ollama-openrouter-proxy)。
