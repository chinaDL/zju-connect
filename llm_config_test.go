package main

import (
	"strings"
	"testing"
)

func TestLLMCaptchaConfiguration(t *testing.T) {
	file := writeConfig(t, "llm_captcha = true\nllm_base_url = \"https://file.example/v1\"\nllm_api_key = \"file-secret\"\nllm_model = \"file-vision\"\n")
	env := []string{"ZJU_CONNECT_LLM_CAPTCHA=true", "ZJU_CONNECT_LLM_BASE_URL=https://env.example/v1", "ZJU_CONNECT_LLM_API_KEY=env-secret", "ZJU_CONNECT_LLM_MODEL=env-vision"}
	for _, tt := range []struct {
		name            string
		args            []string
		env             []string
		enabled         bool
		url, key, model string
	}{
		{name: "default disabled", url: "https://api.openai.com/v1"},
		{name: "file", args: []string{"--config", file}, enabled: true, url: "https://file.example/v1", key: "file-secret", model: "file-vision"},
		{name: "environment over file", args: []string{"--config", file}, env: env, enabled: true, url: "https://env.example/v1", key: "env-secret", model: "env-vision"},
		{name: "single dash over environment", args: []string{"--config", file, "-llm-captcha", "-llm-base-url", "https://cli.example/v1", "-llm-api-key", "cli-secret", "-llm-model", "cli-vision"}, env: env, enabled: true, url: "https://cli.example/v1", key: "cli-secret", model: "cli-vision"},
		{name: "double dash", args: []string{"--llm-captcha", "--llm-api-key=test-secret", "--llm-model=test-vision"}, enabled: true, url: "https://api.openai.com/v1", key: "test-secret", model: "test-vision"},
		{name: "explicit disable", args: []string{"--llm-captcha=false"}, env: env, url: "https://env.example/v1", key: "env-secret", model: "env-vision"},
		{name: "environment disable", args: []string{"--config", file}, env: []string{"ZJU_CONNECT_LLM_CAPTCHA=false"}, url: "https://file.example/v1", key: "file-secret", model: "file-vision"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options, flags, err := loadStartupOptions(tt.args, func() []string { return tt.env })
			if err != nil {
				t.Fatal(err)
			}
			cfg := options.Config
			if cfg.LLMCaptcha != tt.enabled || cfg.LLMBaseURL != tt.url || cfg.LLMAPIKey != tt.key || cfg.LLMModel != tt.model {
				t.Fatal("LLM configuration precedence mismatch")
			}
			if tt.key != "" && strings.Contains(flags.FlagUsages(), tt.key) {
				t.Fatal("API key leaked in help")
			}
		})
	}
}

func TestLLMCaptchaConfigurationValidation(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want string
	}{
		{"missing key", []string{"-llm-captcha", "-llm-model", "vision"}, "llm-api-key"},
		{"missing model", []string{"-llm-captcha", "-llm-api-key", "private-secret"}, "llm-model"},
		{"bad URL", []string{"-llm-captcha", "-llm-api-key", "private-secret", "-llm-model", "vision", "-llm-base-url", "https://user:private-secret@model.example"}, "llm-base-url"},
		{"wrong protocol", []string{"-llm-captcha", "-protocol", "easyconnect", "-llm-api-key", "private-secret", "-llm-model", "vision"}, "atrust"},
		{"bad header", []string{"-llm-captcha", "-llm-api-key", "private-secret\r\nInjected: x", "-llm-model", "vision"}, "llm-api-key"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := loadStartupOptions(tt.args, func() []string { return nil })
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation=%v", err)
			}
			if strings.Contains(err.Error(), "private-secret") {
				t.Fatal("validation leaked API key")
			}
		})
	}
	if _, _, err := loadStartupOptions([]string{"-llm-base-url", "invalid-but-disabled"}, func() []string { return nil }); err != nil {
		t.Fatal(err)
	}
}
