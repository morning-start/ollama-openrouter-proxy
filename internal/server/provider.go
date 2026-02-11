package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
)

type OpenrouterProvider struct {
	client     *openai.Client
	modelNames []string
}

func NewOpenrouterProvider(apiKey string) *OpenrouterProvider {
	config := openai.DefaultConfig(apiKey)
	config.BaseURL = "https://openrouter.ai/api/v1/"

	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}

	return &OpenrouterProvider{
		client:     openai.NewClientWithConfig(config),
		modelNames: []string{},
	}
}

func (o *OpenrouterProvider) Chat(messages []openai.ChatCompletionMessage, modelName string) (openai.ChatCompletionResponse, error) {
	if modelName == "" {
		return openai.ChatCompletionResponse{}, fmt.Errorf("model name cannot be empty")
	}
	if len(messages) == 0 {
		return openai.ChatCompletionResponse{}, fmt.Errorf("messages cannot be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   false,
	}

	resp, err := o.client.CreateChatCompletion(ctx, req)
	if err != nil {
		return openai.ChatCompletionResponse{}, fmt.Errorf("chat completion failed: %w", err)
	}

	return resp, nil
}

func (o *OpenrouterProvider) ChatStream(messages []openai.ChatCompletionMessage, modelName string) (*openai.ChatCompletionStream, error) {
	if modelName == "" {
		return nil, fmt.Errorf("model name cannot be empty")
	}
	if len(messages) == 0 {
		return nil, fmt.Errorf("messages cannot be empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)

	req := openai.ChatCompletionRequest{
		Model:    modelName,
		Messages: messages,
		Stream:   true,
	}

	stream, err := o.client.CreateChatCompletionStream(ctx, req)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("stream creation failed: %w", err)
	}

	return stream, nil
}

type ModelDetails struct {
	ParentModel       string   `json:"parent_model"`
	Format            string   `json:"format"`
	Family            string   `json:"family"`
	Families          []string `json:"families"`
	ParameterSize     string   `json:"parameter_size"`
	QuantizationLevel string   `json:"quantization_level"`
}

type Model struct {
	Name       string       `json:"name"`
	Model      string       `json:"model,omitempty"`
	ModifiedAt string       `json:"modified_at,omitempty"`
	Size       int64        `json:"size,omitempty"`
	Digest     string       `json:"digest,omitempty"`
	Details    ModelDetails `json:"details,omitempty"`
}

func (o *OpenrouterProvider) GetModels() ([]Model, error) {
	currentTime := time.Now().Format(time.RFC3339)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	modelsResponse, err := o.client.ListModels(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list models: %w", err)
	}

	o.modelNames = []string{}

	var models []Model
	for _, apiModel := range modelsResponse.Models {
		parts := strings.Split(apiModel.ID, "/")
		name := parts[len(parts)-1]

		o.modelNames = append(o.modelNames, apiModel.ID)

		model := Model{
			Name:       name,
			Model:      name,
			ModifiedAt: currentTime,
			Size:       0,
			Digest:     name,
			Details: ModelDetails{
				ParentModel:       "",
				Format:            "gguf",
				Family:            "claude",
				Families:          []string{"claude"},
				ParameterSize:     "175B",
				QuantizationLevel: "Q4_K_M",
			},
		}
		models = append(models, model)
	}

	return models, nil
}

func (o *OpenrouterProvider) GetModelDetails(modelName string) (map[string]interface{}, error) {
	currentTime := time.Now().Format(time.RFC3339)
	return map[string]interface{}{
		"license":    "STUB License",
		"system":     "STUB SYSTEM",
		"modifiedAt": currentTime,
		"details": map[string]interface{}{
			"format":             "gguf",
			"parameter_size":     "200B",
			"quantization_level": "Q4_K_M",
		},
		"model_info": map[string]interface{}{
			"architecture":    "STUB",
			"context_length":  200000,
			"parameter_count": 200_000_000_000,
		},
	}, nil
}

func (o *OpenrouterProvider) GetFullModelName(alias string) (string, error) {
	if len(o.modelNames) == 0 {
		_, err := o.GetModels()
		if err != nil {
			return "", fmt.Errorf("failed to get models: %w", err)
		}
	}

	for _, fullName := range o.modelNames {
		if fullName == alias {
			return fullName, nil
		}
	}

	for _, fullName := range o.modelNames {
		if strings.HasSuffix(fullName, alias) {
			return fullName, nil
		}
	}

	return alias, nil
}

// GetEmbeddings 获取文本的嵌入向量
func (o *OpenrouterProvider) GetEmbeddings(input string, model string) ([]float32, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req := openai.EmbeddingRequest{
		Input: []string{input},
		Model: openai.EmbeddingModel(model),
	}

	resp, err := o.client.CreateEmbeddings(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("embeddings creation failed: %w", err)
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return resp.Data[0].Embedding, nil
}
