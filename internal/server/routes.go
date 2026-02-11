package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
)

// setupRoutes 配置所有路由
func (s *Server) setupRoutes(r *gin.Engine) {
	// 根路径和健康检查
	r.GET("/", s.handleRoot)
	r.HEAD("/", s.handleHeadRoot)
	r.GET("/health", s.handleHealth)

	// Ollama API 端点
	r.POST("/api/generate", s.handleGenerate)
	r.POST("/api/chat", s.handleChat)
	r.GET("/api/tags", s.handleListModels)
	r.POST("/api/show", s.handleShowModel)
	r.POST("/api/create", s.handleCreateModel)
	r.POST("/api/copy", s.handleCopyModel)
	r.DELETE("/api/delete", s.handleDeleteModel)
	r.POST("/api/pull", s.handlePullModel)
	r.POST("/api/push", s.handlePushModel)
	r.POST("/api/embeddings", s.handleEmbeddings)
	r.GET("/api/ps", s.handleRunningModels)
	r.GET("/api/version", s.handleVersion)

	// OpenAI 兼容端点
	r.GET("/v1/models", s.handleOpenAIModels)
	r.POST("/v1/chat/completions", s.handleOpenAIChat)
	r.POST("/v1/embeddings", s.handleOpenAIEmbeddings)
}

// handleRoot 处理根路径请求
func (s *Server) handleRoot(c *gin.Context) {
	c.String(http.StatusOK, "Ollama is running")
}

// handleHeadRoot 处理 HEAD 请求
func (s *Server) handleHeadRoot(c *gin.Context) {
	c.Status(http.StatusOK)
}

// handleHealth 健康检查
func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// handleVersion 返回版本信息
func (s *Server) handleVersion(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"version": "0.1.0",
	})
}

// GenerateRequest Ollama Generate API 请求结构
type GenerateRequest struct {
	Model   string   `json:"model" binding:"required"`
	Prompt  string   `json:"prompt" binding:"required"`
	Suffix  string   `json:"suffix,omitempty"`
	System  string   `json:"system,omitempty"`
	Template string  `json:"template,omitempty"`
	Context []int    `json:"context,omitempty"`
	Stream  *bool    `json:"stream,omitempty"`
	Raw     bool     `json:"raw,omitempty"`
	Format  string   `json:"format,omitempty"`
	Options map[string]interface{} `json:"options,omitempty"`
}

// GenerateResponse Ollama Generate API 响应结构
type GenerateResponse struct {
	Model              string `json:"model"`
	CreatedAt          string `json:"created_at"`
	Response           string `json:"response"`
	Done               bool   `json:"done"`
	DoneReason         string `json:"done_reason,omitempty"`
	Context            []int  `json:"context,omitempty"`
	TotalDuration      int64  `json:"total_duration,omitempty"`
	LoadDuration       int64  `json:"load_duration,omitempty"`
	PromptEvalCount    int    `json:"prompt_eval_count,omitempty"`
	PromptEvalDuration int64  `json:"prompt_eval_duration,omitempty"`
	EvalCount          int    `json:"eval_count,omitempty"`
	EvalDuration       int64  `json:"eval_duration,omitempty"`
}

// handleGenerate 处理 /api/generate 请求
func (s *Server) handleGenerate(c *gin.Context) {
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 将 generate 请求转换为 chat 请求
	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: req.Prompt},
	}

	// 如果有 system 提示，添加到消息列表
	if req.System != "" {
		messages = append([]openai.ChatCompletionMessage{
			{Role: "system", Content: req.System},
		}, messages...)
	}

	stream := true
	if req.Stream != nil {
		stream = *req.Stream
	}

	startTime := time.Now()

	if !stream {
		s.handleNonStreamingGenerate(c, req.Model, messages, startTime)
	} else {
		s.handleStreamingGenerate(c, req.Model, messages, startTime)
	}
}

// handleNonStreamingGenerate 处理非流式生成
func (s *Server) handleNonStreamingGenerate(c *gin.Context, model string, messages []openai.ChatCompletionMessage, startTime time.Time) {
	var response openai.ChatCompletionResponse
	var fullModelName string
	var err error

	if s.config.FreeMode {
		response, fullModelName, err = s.getFreeChatForModel(messages, model)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
	} else {
		fullModelName, err = s.provider.GetFullModelName(model)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		response, err = s.provider.Chat(messages, fullModelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	totalDuration := time.Since(startTime).Nanoseconds()

	resp := GenerateResponse{
		Model:              fullModelName,
		CreatedAt:          time.Now().Format(time.RFC3339),
		Response:           response.Choices[0].Message.Content,
		Done:               true,
		DoneReason:         "stop",
		TotalDuration:      totalDuration,
		PromptEvalCount:    response.Usage.PromptTokens,
		EvalCount:          response.Usage.CompletionTokens,
	}

	c.JSON(http.StatusOK, resp)
}

// handleStreamingGenerate 处理流式生成
func (s *Server) handleStreamingGenerate(c *gin.Context, model string, messages []openai.ChatCompletionMessage, startTime time.Time) {
	var stream *openai.ChatCompletionStream
	var fullModelName string
	var err error

	if s.config.FreeMode {
		stream, fullModelName, err = s.getFreeStreamForModel(messages, model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	} else {
		fullModelName, err = s.provider.GetFullModelName(model)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		stream, err = s.provider.ChatStream(messages, fullModelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	defer stream.Close()

	c.Writer.Header().Set("Content-Type", "application/x-ndjson")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Streaming not supported"})
		return
	}

	var fullResponse string
	evalCount := 0

	for {
		response, err := stream.Recv()
		if err != nil {
			break
		}

		if len(response.Choices) > 0 {
			content := response.Choices[0].Delta.Content
			fullResponse += content
			evalCount++

			resp := GenerateResponse{
				Model:     fullModelName,
				CreatedAt: time.Now().Format(time.RFC3339),
				Response:  content,
				Done:      false,
			}

			jsonData, _ := json.Marshal(resp)
			fmt.Fprintf(c.Writer, "%s\n", string(jsonData))
			flusher.Flush()
		}
	}

	totalDuration := time.Since(startTime).Nanoseconds()

	finalResp := GenerateResponse{
		Model:              fullModelName,
		CreatedAt:          time.Now().Format(time.RFC3339),
		Response:           "",
		Done:               true,
		DoneReason:         "stop",
		TotalDuration:      totalDuration,
		EvalCount:          evalCount,
	}

	jsonData, _ := json.Marshal(finalResp)
	fmt.Fprintf(c.Writer, "%s\n", string(jsonData))
	flusher.Flush()
}

// CreateModelRequest 创建模型请求
type CreateModelRequest struct {
	Name      string `json:"name" binding:"required"`
	Modelfile string `json:"modelfile,omitempty"`
	Path      string `json:"path,omitempty"`
}

// handleCreateModel 处理 /api/create 请求
func (s *Server) handleCreateModel(c *gin.Context) {
	var req CreateModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// OpenRouter 不支持创建模型，返回提示
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Model creation is not supported with OpenRouter provider",
	})
}

// CopyModelRequest 复制模型请求
type CopyModelRequest struct {
	Source      string `json:"source" binding:"required"`
	Destination string `json:"destination" binding:"required"`
}

// handleCopyModel 处理 /api/copy 请求
func (s *Server) handleCopyModel(c *gin.Context) {
	var req CopyModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// OpenRouter 不支持复制模型，返回提示
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Model copy is not supported with OpenRouter provider",
	})
}

// DeleteModelRequest 删除模型请求
type DeleteModelRequest struct {
	Name string `json:"name" binding:"required"`
}

// handleDeleteModel 处理 /api/delete 请求
func (s *Server) handleDeleteModel(c *gin.Context) {
	var req DeleteModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// OpenRouter 不支持删除模型，返回提示
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Model deletion is not supported with OpenRouter provider",
	})
}

// PullModelRequest 拉取模型请求
type PullModelRequest struct {
	Name   string `json:"name" binding:"required"`
	Stream *bool  `json:"stream,omitempty"`
}

// handlePullModel 处理 /api/pull 请求
func (s *Server) handlePullModel(c *gin.Context) {
	var req PullModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// OpenRouter 不需要拉取模型，返回提示
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Model pull is not required with OpenRouter provider",
	})
}

// PushModelRequest 推送模型请求
type PushModelRequest struct {
	Name   string `json:"name" binding:"required"`
	Stream *bool  `json:"stream,omitempty"`
}

// handlePushModel 处理 /api/push 请求
func (s *Server) handlePushModel(c *gin.Context) {
	var req PushModelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// OpenRouter 不支持推送模型，返回提示
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"message": "Model push is not supported with OpenRouter provider",
	})
}

// EmbeddingsRequest 嵌入请求
type EmbeddingsRequest struct {
	Model string `json:"model" binding:"required"`
	Prompt string `json:"prompt" binding:"required"`
}

// EmbeddingsResponse 嵌入响应
type EmbeddingsResponse struct {
	Embedding []float32 `json:"embedding"`
}

// handleEmbeddings 处理 /api/embeddings 请求
func (s *Server) handleEmbeddings(c *gin.Context) {
	var req EmbeddingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// OpenRouter 支持嵌入，调用相应接口
	embedding, err := s.provider.GetEmbeddings(req.Prompt, req.Model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, EmbeddingsResponse{
		Embedding: embedding,
	})
}

// RunningModelsResponse 运行中模型响应
type RunningModelsResponse struct {
	Models []RunningModel `json:"models"`
}

// RunningModel 运行中的模型
type RunningModel struct {
	Name       string    `json:"name"`
	Model      string    `json:"model"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	Details    ModelDetails `json:"details"`
	ExpiresAt  time.Time `json:"expires_at"`
	SizeVRAM   int64     `json:"size_vram"`
}

// handleRunningModels 处理 /api/ps 请求
func (s *Server) handleRunningModels(c *gin.Context) {
	// 由于 OpenRouter 是无状态服务，返回空列表
	c.JSON(http.StatusOK, RunningModelsResponse{
		Models: []RunningModel{},
	})
}

// OpenAIEmbeddingsRequest OpenAI Embeddings API 请求
type OpenAIEmbeddingsRequest struct {
	Model string `json:"model" binding:"required"`
	Input string `json:"input" binding:"required"`
}

// OpenAIEmbeddingsResponse OpenAI Embeddings API 响应
type OpenAIEmbeddingsResponse struct {
	Object string          `json:"object"`
	Data   []EmbeddingData `json:"data"`
	Model  string          `json:"model"`
	Usage  EmbeddingUsage  `json:"usage"`
}

// EmbeddingData 嵌入数据
type EmbeddingData struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// EmbeddingUsage 嵌入使用统计
type EmbeddingUsage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// handleOpenAIEmbeddings 处理 OpenAI Embeddings API 请求
func (s *Server) handleOpenAIEmbeddings(c *gin.Context) {
	var req OpenAIEmbeddingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	embedding, err := s.provider.GetEmbeddings(req.Input, req.Model)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}

	resp := OpenAIEmbeddingsResponse{
		Object: "list",
		Data: []EmbeddingData{
			{
				Object:    "embedding",
				Embedding: embedding,
				Index:     0,
			},
		},
		Model: req.Model,
		Usage: EmbeddingUsage{
			PromptTokens: len(req.Input),
			TotalTokens:  len(req.Input),
		},
	}

	c.JSON(http.StatusOK, resp)
}
