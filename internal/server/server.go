package server

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sashabaranov/go-openai"
)

type Config struct {
	APIKey      string
	Host        string
	Port        string
	FreeMode    bool
	ToolUseOnly bool
	ConfigDir   string
	FilterPath  string
	LogLevel    string
}

type Server struct {
	config          Config
	httpServer      *http.Server
	provider        *OpenrouterProvider
	failureStore    *FailureStore
	globalLimiter   *GlobalRateLimiter
	permanentFails  *PermanentFailureTracker
	freeModels      []string
	modelFilter     map[string]struct{}
}

func New(cfg Config) *Server {
	return &Server{
		config:         cfg,
		modelFilter:    make(map[string]struct{}),
		globalLimiter:  NewGlobalRateLimiter(),
		permanentFails: NewPermanentFailureTracker(),
	}
}

func (s *Server) Start() error {
	s.provider = NewOpenrouterProvider(s.config.APIKey)

	if s.config.FreeMode {
		if err := s.initFreeMode(); err != nil {
			return err
		}
	}

	s.loadModelFilter()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	s.setupRoutes(r)

	s.httpServer = &http.Server{
		Addr:         s.config.Host + ":" + s.config.Port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s.failureStore != nil {
		s.failureStore.Close()
	}
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) initFreeMode() error {
	cacheFile := filepath.Join(s.config.ConfigDir, "free-models")
	os.Setenv("FREE_MODELS_CACHE", cacheFile)

	models, err := s.ensureFreeModelFile(s.config.APIKey, cacheFile)
	if err != nil {
		return fmt.Errorf("failed to load free models: %w", err)
	}
	s.freeModels = models

	dbFile := filepath.Join(s.config.ConfigDir, "failures.db")
	os.Setenv("FAILURE_DB", dbFile)

	failureStore, err := NewFailureStore(dbFile)
	if err != nil {
		return fmt.Errorf("failed to init failure store: %w", err)
	}
	s.failureStore = failureStore

	slog.Info("Free mode enabled", "models", len(s.freeModels))
	return nil
}

func (s *Server) loadModelFilter() {
	file, err := os.Open(s.config.FilterPath)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Error("Error loading model filter", "error", err)
		}
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			s.modelFilter[line] = struct{}{}
		}
	}

	slog.Info("Model filter loaded", "patterns", len(s.modelFilter))
}



func (s *Server) handleListModels(c *gin.Context) {
	var newModels []map[string]interface{}
	toolUseOnly := strings.ToLower(os.Getenv("TOOL_USE_ONLY")) == "true"
	currentTime := time.Now().Format(time.RFC3339)

	if s.config.FreeMode {
		for _, freeModel := range s.freeModels {
			skip, err := s.failureStore.ShouldSkip(freeModel)
			if err != nil {
				slog.Error("db error checking model", "model", freeModel, "error", err)
				continue
			}
			if skip {
				continue
			}

			parts := strings.Split(freeModel, "/")
			displayName := parts[len(parts)-1]

			if !s.isModelInFilter(displayName) {
				continue
			}

			newModels = append(newModels, map[string]interface{}{
				"name":        displayName,
				"model":       displayName,
				"modified_at": currentTime,
				"size":        270898672,
				"digest":      "9077fe9d2ae1a4a41a868836b56b8163731a8fe16621397028c2c76f838c6907",
				"details": map[string]interface{}{
					"parent_model":       "",
					"format":             "gguf",
					"family":             "free",
					"families":           []string{"free"},
					"parameter_size":     "varies",
					"quantization_level": "Q4_K_M",
				},
			})
		}
	} else {
		if toolUseOnly {
			newModels = s.fetchToolUseModels(c)
			if newModels == nil {
				return
			}
		} else {
			models, err := s.provider.GetModels()
			if err != nil {
				slog.Error("Error getting models", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
			newModels = make([]map[string]interface{}, 0, len(models))
			for _, m := range models {
				if len(s.modelFilter) > 0 {
					if _, ok := s.modelFilter[m.Model]; !ok {
						continue
					}
				}
				newModels = append(newModels, map[string]interface{}{
					"name":        m.Name,
					"model":       m.Model,
					"modified_at": m.ModifiedAt,
					"size":        270898672,
					"digest":      "9077fe9d2ae1a4a41a868836b56b8163731a8fe16621397028c2c76f838c6907",
					"details":     m.Details,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"models": newModels})
}

func (s *Server) isModelInFilter(modelName string) bool {
	if len(s.modelFilter) == 0 {
		return true
	}
	for pattern := range s.modelFilter {
		if strings.Contains(modelName, pattern) {
			return true
		}
	}
	return false
}

func (s *Server) fetchToolUseModels(c *gin.Context) []map[string]interface{} {
	req, err := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		slog.Error("Error creating request", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		slog.Error("Error fetching models", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Error("Unexpected status", "status", resp.Status)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch models"})
		return nil
	}

	var result orModels
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		slog.Error("Error decoding response", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return nil
	}

	currentTime := time.Now().Format(time.RFC3339)
	newModels := make([]map[string]interface{}, 0)
	for _, m := range result.Data {
		if !supportsToolUse(m.SupportedParameters) {
			continue
		}

		parts := strings.Split(m.ID, "/")
		displayName := parts[len(parts)-1]

		if !s.isModelInFilter(displayName) {
			continue
		}

		newModels = append(newModels, map[string]interface{}{
			"name":        displayName,
			"model":       displayName,
			"modified_at": currentTime,
			"size":        270898672,
			"digest":      "9077fe9d2ae1a4a41a868836b56b8163731a8fe16621397028c2c76f838c6907",
			"details": map[string]interface{}{
				"parent_model":       "",
				"format":             "gguf",
				"family":             "tool-enabled",
				"families":           []string{"tool-enabled"},
				"parameter_size":     "varies",
				"quantization_level": "Q4_K_M",
			},
		})
	}
	return newModels
}

func (s *Server) handleShowModel(c *gin.Context) {
	var request map[string]string
	if err := c.BindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	modelName := request["name"]
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Model name is required"})
		return
	}

	details, err := s.provider.GetModelDetails(modelName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, details)
}

func (s *Server) handleChat(c *gin.Context) {
	var request struct {
		Model    string                         `json:"model"`
		Messages []openai.ChatCompletionMessage `json:"messages"`
		Stream   *bool                          `json:"stream"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		slog.Warn("Invalid JSON", "error", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON: " + err.Error()})
		return
	}

	if request.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Model name is required"})
		return
	}
	if len(request.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Messages cannot be empty"})
		return
	}

	streamRequested := true
	if request.Stream != nil {
		streamRequested = *request.Stream
	}

	if !streamRequested {
		s.handleNonStreamingChat(c, request.Model, request.Messages)
	} else {
		s.handleStreamingChat(c, request.Model, request.Messages)
	}
}

func (s *Server) handleNonStreamingChat(c *gin.Context, model string, messages []openai.ChatCompletionMessage) {
	var response openai.ChatCompletionResponse
	var fullModelName string
	var err error

	if s.config.FreeMode {
		response, fullModelName, err = s.getFreeChatForModel(messages, model)
		if err != nil {
			slog.Error("free mode failed", "error", err)
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

	if len(response.Choices) == 0 {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "No response"})
		return
	}

	content := response.Choices[0].Message.Content
	finishReason := "stop"
	if response.Choices[0].FinishReason != "" {
		finishReason = string(response.Choices[0].FinishReason)
	}

	c.JSON(http.StatusOK, map[string]interface{}{
		"model":      fullModelName,
		"created_at": time.Now().Format(time.RFC3339),
		"message": map[string]string{
			"role":    "assistant",
			"content": content,
		},
		"done":              true,
		"finish_reason":     finishReason,
		"total_duration":    response.Usage.TotalTokens * 10,
		"load_duration":     0,
		"prompt_eval_count": response.Usage.PromptTokens,
		"eval_count":        response.Usage.CompletionTokens,
		"eval_duration":     response.Usage.CompletionTokens * 10,
	})
}

func (s *Server) handleStreamingChat(c *gin.Context, model string, messages []openai.ChatCompletionMessage) {
	var stream *openai.ChatCompletionStream
	var fullModelName string
	var err error

	if s.config.FreeMode {
		stream, fullModelName, err = s.getFreeStreamForModel(messages, model)
		if err != nil {
			slog.Error("free mode failed", "error", err)
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

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	var lastFinishReason string

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			errorMsg := map[string]string{"error": "Stream error: " + err.Error()}
			errorJson, _ := json.Marshal(errorMsg)
			fmt.Fprintf(w, "%s\n", string(errorJson))
			flusher.Flush()
			return
		}

		if len(response.Choices) > 0 && response.Choices[0].FinishReason != "" {
			lastFinishReason = string(response.Choices[0].FinishReason)
		}

		responseJSON := map[string]interface{}{
			"model":      fullModelName,
			"created_at": time.Now().Format(time.RFC3339),
			"message": map[string]string{
				"role":    "assistant",
				"content": response.Choices[0].Delta.Content,
			},
			"done": false,
		}

		jsonData, _ := json.Marshal(responseJSON)
		fmt.Fprintf(w, "%s\n", string(jsonData))
		flusher.Flush()
	}

	if lastFinishReason == "" {
		lastFinishReason = "stop"
	}

	finalResponse := map[string]interface{}{
		"model":      fullModelName,
		"created_at": time.Now().Format(time.RFC3339),
		"message": map[string]string{
			"role":    "assistant",
			"content": "",
		},
		"done":              true,
		"finish_reason":     lastFinishReason,
		"total_duration":    0,
		"load_duration":     0,
		"prompt_eval_count": 0,
		"eval_count":        0,
		"eval_duration":     0,
	}

	finalJsonData, _ := json.Marshal(finalResponse)
	fmt.Fprintf(w, "%s\n", string(finalJsonData))
	flusher.Flush()
}

func (s *Server) handleOpenAIChat(c *gin.Context) {
	var request openai.ChatCompletionRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON"})
		return
	}

	if request.Stream {
		s.handleOpenAIStreaming(c, request)
	} else {
		s.handleOpenAINonStreaming(c, request)
	}
}

func (s *Server) handleOpenAIStreaming(c *gin.Context, request openai.ChatCompletionRequest) {
	var stream *openai.ChatCompletionStream
	var fullModelName string
	var err error

	if s.config.FreeMode {
		stream, fullModelName, err = s.getFreeStreamForModel(request.Messages, request.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
			return
		}
	} else {
		fullModelName, err = s.provider.GetFullModelName(request.Model)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": err.Error()}})
			return
		}
		stream, err = s.provider.ChatStream(request.Messages, fullModelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
			return
		}
	}
	defer stream.Close()

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	for {
		response, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			break
		}
		if err != nil {
			break
		}

		openaiResponse := openai.ChatCompletionStreamResponse{
			ID:      "chatcmpl-" + fmt.Sprintf("%d", time.Now().Unix()),
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   fullModelName,
			Choices: []openai.ChatCompletionStreamChoice{
				{
					Index: 0,
					Delta: openai.ChatCompletionStreamChoiceDelta{
						Content: response.Choices[0].Delta.Content,
					},
				},
			},
		}

		if len(response.Choices) > 0 && response.Choices[0].FinishReason != "" {
			openaiResponse.Choices[0].FinishReason = response.Choices[0].FinishReason
		}

		jsonData, _ := json.Marshal(openaiResponse)
		fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
		flusher.Flush()
	}
}

func (s *Server) handleOpenAINonStreaming(c *gin.Context, request openai.ChatCompletionRequest) {
	var response openai.ChatCompletionResponse
	var fullModelName string
	var err error

	if s.config.FreeMode {
		response, fullModelName, err = s.getFreeChatForModel(request.Messages, request.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
			return
		}
	} else {
		fullModelName, err = s.provider.GetFullModelName(request.Model)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": err.Error()}})
			return
		}
		response, err = s.provider.Chat(request.Messages, fullModelName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
			return
		}
	}

	response.ID = "chatcmpl-" + fmt.Sprintf("%d", time.Now().Unix())
	response.Object = "chat.completion"
	response.Created = time.Now().Unix()
	response.Model = fullModelName

	c.JSON(http.StatusOK, response)
}

func (s *Server) handleOpenAIModels(c *gin.Context) {
	var models []gin.H
	toolUseOnly := strings.ToLower(os.Getenv("TOOL_USE_ONLY")) == "true"

	if s.config.FreeMode {
		for _, freeModel := range s.freeModels {
			skip, err := s.failureStore.ShouldSkip(freeModel)
			if err != nil {
				continue
			}
			if skip {
				continue
			}

			parts := strings.Split(freeModel, "/")
			displayName := parts[len(parts)-1]

			if !s.isModelInFilter(displayName) {
				continue
			}

			models = append(models, gin.H{
				"id":       displayName,
				"object":   "model",
				"created":  time.Now().Unix(),
				"owned_by": "openrouter",
			})
		}
	} else {
		if toolUseOnly {
			models = s.fetchOpenAIToolUseModels(c)
			if models == nil {
				return
			}
		} else {
			providerModels, err := s.provider.GetModels()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
				return
			}

			for _, m := range providerModels {
				if len(s.modelFilter) > 0 {
					if _, ok := s.modelFilter[m.Model]; !ok {
						continue
					}
				}
				models = append(models, gin.H{
					"id":       m.Model,
					"object":   "model",
					"created":  time.Now().Unix(),
					"owned_by": "openrouter",
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   models,
	})
}

func (s *Server) fetchOpenAIToolUseModels(c *gin.Context) []gin.H {
	req, err := http.NewRequest("GET", "https://openrouter.ai/api/v1/models", nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
		return nil
	}
	req.Header.Set("Authorization", "Bearer "+s.config.APIKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to fetch models"}})
		return nil
	}

	var result orModels
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": err.Error()}})
		return nil
	}

	var models []gin.H
	for _, m := range result.Data {
		if !supportsToolUse(m.SupportedParameters) {
			continue
		}

		parts := strings.Split(m.ID, "/")
		displayName := parts[len(parts)-1]

		if !s.isModelInFilter(displayName) {
			continue
		}

		models = append(models, gin.H{
			"id":       displayName,
			"object":   "model",
			"created":  time.Now().Unix(),
			"owned_by": "openrouter",
		})
	}
	return models
}

func (s *Server) getFreeChatForModel(msgs []openai.ChatCompletionMessage, requestedModel string) (openai.ChatCompletionResponse, string, error) {
	fullModelName := s.resolveDisplayNameToFullModel(requestedModel)
	if fullModelName != requestedModel || s.contains(s.freeModels, fullModelName) {
		skip, err := s.failureStore.ShouldSkip(fullModelName)
		if err == nil && !skip {
			resp, err := s.provider.Chat(msgs, fullModelName)
			if err == nil {
				s.failureStore.ClearFailure(fullModelName)
				return resp, fullModelName, nil
			}
			s.failureStore.MarkFailure(fullModelName)
		}
	}
	return s.getFreeChat(msgs)
}

func (s *Server) getFreeStreamForModel(msgs []openai.ChatCompletionMessage, requestedModel string) (*openai.ChatCompletionStream, string, error) {
	fullModelName := s.resolveDisplayNameToFullModel(requestedModel)
	if fullModelName != requestedModel || s.contains(s.freeModels, fullModelName) {
		skip, err := s.failureStore.ShouldSkip(fullModelName)
		if err == nil && !skip {
			stream, err := s.provider.ChatStream(msgs, fullModelName)
			if err == nil {
				s.failureStore.ClearFailure(fullModelName)
				return stream, fullModelName, nil
			}
			s.failureStore.MarkFailure(fullModelName)
		}
	}
	return s.getFreeStream(msgs)
}

func (s *Server) getFreeChat(msgs []openai.ChatCompletionMessage) (openai.ChatCompletionResponse, string, error) {
	var resp openai.ChatCompletionResponse
	var lastError error

	for _, m := range s.freeModels {
		if s.permanentFails.IsPermanentlyFailed(m) {
			continue
		}

		parts := strings.Split(m, "/")
		displayName := parts[len(parts)-1]
		if !s.isModelInFilter(displayName) {
			continue
		}

		skip, err := s.failureStore.ShouldSkip(m)
		if err != nil || skip {
			continue
		}

		limiter := s.globalLimiter.GetLimiter(m)
		limiter.Wait()
		s.globalLimiter.WaitGlobal()

		resp, err = s.provider.Chat(msgs, m)
		if err != nil {
			lastError = err
			limiter.RecordFailure(err)

			if isPermanentError(err) {
				s.permanentFails.MarkPermanentFailure(m)
			} else if isRateLimitError(err) {
				s.failureStore.MarkFailureWithType(m, "rate_limit")
				time.Sleep(500 * time.Millisecond)
			} else {
				s.failureStore.MarkFailure(m)
			}
			continue
		}

		limiter.RecordSuccess()
		s.failureStore.ClearFailure(m)
		return resp, m, nil
	}

	if lastError != nil {
		return resp, "", fmt.Errorf("all models failed: %w", lastError)
	}
	return resp, "", fmt.Errorf("no free models available")
}

func (s *Server) getFreeStream(msgs []openai.ChatCompletionMessage) (*openai.ChatCompletionStream, string, error) {
	var lastError error

	for _, m := range s.freeModels {
		if s.permanentFails.IsPermanentlyFailed(m) {
			continue
		}

		parts := strings.Split(m, "/")
		displayName := parts[len(parts)-1]
		if !s.isModelInFilter(displayName) {
			continue
		}

		skip, err := s.failureStore.ShouldSkip(m)
		if err != nil || skip {
			continue
		}

		limiter := s.globalLimiter.GetLimiter(m)
		limiter.Wait()
		s.globalLimiter.WaitGlobal()

		stream, err := s.provider.ChatStream(msgs, m)
		if err != nil {
			lastError = err
			limiter.RecordFailure(err)

			if isPermanentError(err) {
				s.permanentFails.MarkPermanentFailure(m)
			} else if isRateLimitError(err) {
				s.failureStore.MarkFailureWithType(m, "rate_limit")
				time.Sleep(500 * time.Millisecond)
			} else {
				s.failureStore.MarkFailure(m)
			}
			continue
		}

		limiter.RecordSuccess()
		s.failureStore.ClearFailure(m)
		return stream, m, nil
	}

	if lastError != nil {
		return nil, "", fmt.Errorf("all models failed: %w", lastError)
	}
	return nil, "", fmt.Errorf("no free models available")
}

func (s *Server) resolveDisplayNameToFullModel(displayName string) string {
	for _, fullModel := range s.freeModels {
		parts := strings.Split(fullModel, "/")
		modelDisplayName := parts[len(parts)-1]
		if modelDisplayName == displayName {
			if !s.isModelInFilter(displayName) {
				continue
			}
			return fullModel
		}
	}
	return displayName
}

func (s *Server) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (s *Server) ensureFreeModelFile(apiKey, path string) ([]string, error) {
	cacheTTL := 24 * time.Hour
	if ttlStr := os.Getenv("CACHE_TTL_HOURS"); ttlStr != "" {
		if hours, err := time.ParseDuration(ttlStr + "h"); err == nil {
			cacheTTL = hours
		}
	}

	if stat, err := os.Stat(path); err == nil {
		if time.Since(stat.ModTime()) < cacheTTL {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			var models []string
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line != "" {
					models = append(models, line)
				}
			}
			return models, nil
		}
	}

	models, err := s.fetchFreeModels(apiKey)
	if err != nil {
		if _, statErr := os.Stat(path); statErr == nil {
			data, readErr := os.ReadFile(path)
			if readErr == nil {
				var cachedModels []string
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if line != "" {
						cachedModels = append(cachedModels, line)
					}
				}
				return cachedModels, nil
			}
		}
		return nil, err
	}

	_ = os.WriteFile(path, []byte(strings.Join(models, "\n")), 0644)
	return models, nil
}

func (s *Server) fetchFreeModels(apiKey string) ([]string, error) {
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

	var result orModels
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	toolUseOnly := strings.ToLower(os.Getenv("TOOL_USE_ONLY")) == "true"

	type item struct {
		id  string
		ctx int
	}
	var items []item
	for _, m := range result.Data {
		if m.Pricing.Prompt == "0" && m.Pricing.Completion == "0" {
			if toolUseOnly && !supportsToolUse(m.SupportedParameters) {
				continue
			}

			ctx := m.TopProvider.ContextLength
			if ctx == 0 {
				ctx = m.ContextLength
			}
			items = append(items, item{id: m.ID, ctx: ctx})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ctx > items[j].ctx })
	models := make([]string, len(items))
	for i, it := range items {
		models[i] = it.id
	}
	return models, nil
}
