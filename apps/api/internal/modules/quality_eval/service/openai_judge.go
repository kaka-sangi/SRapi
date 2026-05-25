package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	qualitycontract "github.com/srapi/srapi/apps/api/internal/modules/quality_eval/contract"
)

const (
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	defaultJudgeTimeout  = 20 * time.Second
)

type OpenAIJudgeConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Timeout time.Duration
	Client  *http.Client
}

type OpenAIJudge struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewOpenAIJudge(cfg OpenAIJudgeConfig) (*OpenAIJudge, error) {
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		return nil, ErrUnavailable
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultOpenAIBaseURL
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = qualitycontract.DefaultJudgeModel
	}
	client := cfg.Client
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultJudgeTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	return &OpenAIJudge{apiKey: apiKey, baseURL: baseURL, model: model, client: client}, nil
}

func (j *OpenAIJudge) Evaluate(ctx context.Context, sample qualitycontract.EvaluationSample) (qualitycontract.JudgeResult, error) {
	if j == nil || strings.TrimSpace(sample.SanitizedPrompt) == "" || strings.TrimSpace(sample.SanitizedOutput) == "" {
		return qualitycontract.JudgeResult{}, ErrInvalidInput
	}
	body := chatCompletionRequest{
		Model: j.model,
		Messages: []chatMessage{
			{Role: "system", Content: judgeSystemPrompt},
			{Role: "user", Content: judgeUserPrompt(sample)},
		},
		Temperature:    floatPtr(0),
		ResponseFormat: map[string]string{"type": "json_object"},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return qualitycontract.JudgeResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.baseURL+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return qualitycontract.JudgeResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+j.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := j.client.Do(req)
	if err != nil {
		return qualitycontract.JudgeResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return qualitycontract.JudgeResult{}, fmt.Errorf("quality judge upstream status %d", resp.StatusCode)
	}
	var parsed chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return qualitycontract.JudgeResult{}, err
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return qualitycontract.JudgeResult{}, ErrUnavailable
	}
	return parseJudgeContent(parsed.Model, parsed.Choices[0].Message.Content)
}

type chatCompletionRequest struct {
	Model          string            `json:"model"`
	Messages       []chatMessage     `json:"messages"`
	Temperature    *float64          `json:"temperature,omitempty"`
	ResponseFormat map[string]string `json:"response_format,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type judgeJSON struct {
	Correctness int    `json:"correctness"`
	Coherence   int    `json:"coherence"`
	Safety      int    `json:"safety"`
	Rationale   string `json:"rationale"`
}

const judgeSystemPrompt = "You are an impartial API response quality judge. Return JSON only with integer correctness, coherence, safety fields from 0 to 5 and a short rationale."

func judgeUserPrompt(sample qualitycontract.EvaluationSample) string {
	return "Evaluate this sanitized model output against the sanitized user request.\n\nRequest:\n" +
		sample.SanitizedPrompt + "\n\nOutput:\n" + sample.SanitizedOutput
}

func parseJudgeContent(model string, content string) (qualitycontract.JudgeResult, error) {
	var parsed judgeJSON
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		return qualitycontract.JudgeResult{}, err
	}
	correctness := clampRubric(parsed.Correctness)
	coherence := clampRubric(parsed.Coherence)
	safety := clampRubric(parsed.Safety)
	score := float64(correctness+coherence+safety) / 15
	judgeModel := strings.TrimSpace(model)
	if judgeModel == "" {
		judgeModel = qualitycontract.DefaultJudgeModel
	}
	return qualitycontract.JudgeResult{
		JudgeModel:  judgeModel,
		Score:       clamp01(score),
		Correctness: correctness,
		Coherence:   coherence,
		Safety:      safety,
		Rationale:   parsed.Rationale,
		JudgedAt:    time.Now().UTC(),
	}, nil
}

func floatPtr(value float64) *float64 {
	return &value
}
