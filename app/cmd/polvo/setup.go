package main

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/co2-lab/polvo/internal/config"
	"github.com/co2-lab/polvo/internal/provider"
)

type providerDef struct {
	id           string
	label        string
	providerType string
	needsAPIKey  bool
	needsBaseURL bool
	defaultModel string
	apiKeyHint   string
}

var providerDefs = []providerDef{
	{id: "claude", label: "Claude (Anthropic)", providerType: "claude", needsAPIKey: true, defaultModel: "claude-3-5-sonnet-latest", apiKeyHint: "console.anthropic.com"},
	{id: "gemini", label: "Gemini (Google)", providerType: "gemini", needsAPIKey: true, defaultModel: "gemini-2.0-pro-exp-02-05", apiKeyHint: "aistudio.google.com"},
	{id: "openai", label: "OpenAI", providerType: "openai", needsAPIKey: true, defaultModel: "gpt-4o", apiKeyHint: "platform.openai.com"},
	{id: "deepseek", label: "DeepSeek", providerType: "deepseek", needsAPIKey: true, defaultModel: "deepseek-chat", apiKeyHint: "platform.deepseek.com"},
	{id: "groq", label: "Groq", providerType: "groq", needsAPIKey: true, defaultModel: "llama-3.3-70b-versatile", apiKeyHint: "console.groq.com"},
	{id: "mistral", label: "Mistral AI", providerType: "mistral", needsAPIKey: true, defaultModel: "mistral-large-latest", apiKeyHint: "console.mistral.ai"},
	{id: "openrouter", label: "OpenRouter", providerType: "openrouter", needsAPIKey: true, defaultModel: "anthropic/claude-3.5-sonnet", apiKeyHint: "openrouter.ai"},
	{id: "xai", label: "xAI (Grok)", providerType: "xai", needsAPIKey: true, defaultModel: "grok-3", apiKeyHint: "console.x.ai"},
	{id: "glm", label: "GLM (Zhipu AI)", providerType: "glm", needsAPIKey: true, defaultModel: "glm-4-flash", apiKeyHint: "open.bigmodel.cn"},
	{id: "minimax", label: "MiniMax", providerType: "minimax", needsAPIKey: true, defaultModel: "abab6.5s-chat", apiKeyHint: "platform.minimaxi.com"},
	{id: "kimi", label: "Kimi (Moonshot)", providerType: "kimi", needsAPIKey: true, defaultModel: "moonshot-v1-8k", apiKeyHint: "platform.moonshot.cn"},
	{id: "litellm", label: "LiteLLM (200+ models proxy)", providerType: "openai-compatible", needsAPIKey: false, needsBaseURL: true, defaultModel: "gpt-4o"},
	{id: "openai-compatible", label: "OpenAI-compatible (custom)", providerType: "openai-compatible", needsAPIKey: true, needsBaseURL: true, defaultModel: "gpt-4o"},
	{id: "ollama", label: "Ollama (Local)", providerType: "ollama", needsAPIKey: false, needsBaseURL: true, defaultModel: "llama3.2"},
}

func ask(r *bufio.Reader, label, hint, defaultVal string) (string, error) {
	if hint != "" {
		fmt.Printf("  %s\n", dim(hint))
	}
	if defaultVal != "" {
		fmt.Printf("  %s [%s]: ", label, dim(defaultVal))
	} else {
		fmt.Printf("  %s: ", label)
	}
	val, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	val = strings.TrimSpace(val)
	if val == "" {
		return defaultVal, nil
	}
	return val, nil
}

func askSecret(r *bufio.Reader, label, hint string) (string, error) {
	if hint != "" {
		fmt.Printf("  %s\n", dim(hint))
	}
	fmt.Printf("  %s: ", label)
	val, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(val), nil
}

func runSetupWizard(firstTime bool) (map[string]any, error) {
	r := bufio.NewReader(os.Stdin)

	// ── Header (Ultra-Compact) ────────────────────────────────────────────────
	fmt.Println()
	if firstTime {
		fmt.Printf("%s %s\n", bold("polvo setup"), dim("— Configure your LLM provider"))
	} else {
		fmt.Println(bold("Add new provider"))
	}

	step := 1
	fmt.Printf("\n%s\n", bold(fmt.Sprintf("Step %d:", step)))
	step++

	labels := make([]string, len(providerDefs))
	for i, p := range providerDefs {
		labels[i] = p.label
	}
	pIdx, err := arrowSelect("Choose Provider", labels, 0)
	if err != nil {
		return nil, err
	}
	pdef := providerDefs[pIdx]

	fmt.Printf("\n%s\n", bold(fmt.Sprintf("Step %d:", step)))
	step++
	alias, _ := ask(r, "Alias", "ID for this provider in config", pdef.id)

	apiKey := ""
	if pdef.needsAPIKey {
		fmt.Printf("\n%s\n", bold(fmt.Sprintf("Step %d:", step)))
		step++
		apiKey, err = askSecret(r, "API Key", pdef.apiKeyHint)
		if err != nil || apiKey == "" {
			return nil, fmt.Errorf("api key required")
		}
	}

	baseURL := ""
	if pdef.needsBaseURL {
		fmt.Printf("\n%s\n", bold(fmt.Sprintf("Step %d:", step)))
		step++
		defaultURL := "http://localhost:11434"
		urlHint := "Ollama API URL"
		if pdef.id == "litellm" {
			defaultURL = "http://localhost:4000/v1"
			urlHint = "LiteLLM proxy URL"
			if isLiteLLMRunning() {
				fmt.Printf("  %s\n", "\033[32m✓ LiteLLM detected at localhost:4000\033[0m")
			} else {
				fmt.Printf("  %s\n", dim("LiteLLM not detected. Start it with:"))
				fmt.Printf("  %s\n", dim("  pip install litellm && litellm --model ollama/llama3"))
				fmt.Printf("  %s\n", dim("  or: litellm --config litellm_config.yaml"))
				fmt.Printf("  %s\n", dim("  Docs: docs.litellm.ai/docs/proxy/quick_start"))
			}
		} else if pdef.providerType == "openai-compatible" {
			defaultURL = ""
			urlHint = "Base URL (e.g. https://api.example.com/v1)"
		}
		baseURL, _ = ask(r, "Base URL", urlHint, defaultURL)
	}

	fmt.Printf("\n%s\n", bold(fmt.Sprintf("Step %d:", step)))
	model, err := pickModelDynamic(r, pdef, apiKey, baseURL)
	if err != nil {
		return nil, err
	}

	// ── Save ──────────────────────────────────────────────────────────────────
	providerEntry := map[string]any{"type": pdef.providerType, "default_model": model}
	if apiKey != "" {
		providerEntry["api_key"] = apiKey
	}
	if baseURL != "" {
		providerEntry["base_url"] = baseURL
	}

	cfgPath := config.UserConfigPath()
	_ = os.MkdirAll(filepath.Dir(cfgPath), 0o700)

	// Merge into existing config so we don't overwrite other providers.
	cfgMap := map[string]any{}
	if existing, err := os.ReadFile(cfgPath); err == nil {
		_ = yaml.Unmarshal(existing, &cfgMap)
	}
	providers, _ := cfgMap["providers"].(map[string]any)
	if providers == nil {
		providers = map[string]any{}
	}
	providers[alias] = providerEntry
	cfgMap["providers"] = providers

	out, _ := yaml.Marshal(cfgMap)
	_ = os.WriteFile(cfgPath, out, 0o600)

	fmt.Printf("\n%s %s\n", bold("✓ Config saved:"), cfgPath)
	return cfgMap, nil
}

func pickModelDynamic(r *bufio.Reader, pdef providerDef, apiKey, baseURL string) (string, error) {
	fmt.Printf("%s", dim("Fetching models…"))
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	models, fetchErr := fetchModels(ctx, pdef, apiKey, baseURL)
	fmt.Printf("\r%s\r", strings.Repeat(" ", 20))

	if fetchErr != nil || len(models) == 0 {
		val, _ := ask(r, "Model", "Enter name manually", pdef.defaultModel)
		return val, nil
	}

	labels := make([]string, len(models)+1)
	details := make([]string, len(models)+1)
	for i, m := range models {
		labels[i] = m.ID
		details[i] = modelDetail(m)
	}
	labels[len(models)] = "⌨  Custom model…"

	defaultIdx := 0
	for i, m := range models {
		if m.ID == pdef.defaultModel {
			defaultIdx = i
			break
		}
	}

	idx, err := arrowSelectWithDetails("Choose Model", labels, details, defaultIdx)
	if err != nil {
		return "", err
	}

	if idx == len(models) {
		fmt.Print("  Model Name: ")
		val, _ := r.ReadString('\n')
		val = strings.TrimSpace(val)
		if val == "" {
			return pdef.defaultModel, nil
		}
		return val, nil
	}
	return models[idx].ID, nil
}

func fetchModels(ctx context.Context, pdef providerDef, apiKey, baseURL string) ([]provider.ModelInfo, error) {
	var p provider.ModelLister
	switch pdef.providerType {
	case "openai":
		p = provider.NewOpenAI(pdef.id, apiKey, baseURL, pdef.defaultModel)
	case "gemini":
		p = provider.NewGemini(pdef.id, apiKey, pdef.defaultModel)
	case "deepseek":
		p = provider.NewDeepSeek(pdef.id, apiKey, pdef.defaultModel)
	case "groq":
		p = provider.NewGroq(pdef.id, apiKey, pdef.defaultModel)
	case "mistral":
		p = provider.NewMistral(pdef.id, apiKey, pdef.defaultModel)
	case "openrouter":
		p = provider.NewOpenRouter(pdef.id, apiKey, pdef.defaultModel)
	case "xai":
		p = provider.NewXAI(pdef.id, apiKey, pdef.defaultModel)
	case "glm":
		p = provider.NewGLM(pdef.id, apiKey, pdef.defaultModel)
	case "minimax":
		p = provider.NewMiniMax(pdef.id, apiKey, pdef.defaultModel)
	case "kimi":
		p = provider.NewKimi(pdef.id, apiKey, pdef.defaultModel)
	case "openai-compatible":
		p = provider.NewOpenAI(pdef.id, apiKey, baseURL, pdef.defaultModel)
	case "ollama":
		base := baseURL
		if base == "" {
			base = "http://localhost:11434"
		}
		p = provider.NewOpenAI(pdef.id, "", base+"/v1", pdef.defaultModel)
	default:
		return nil, fmt.Errorf("unsupported")
	}
	return p.ListModels(ctx)
}

// modelDetail returns a compact one-line string with benchmark scores for a model.
// Empty string when no scores are known.
func modelDetail(m provider.ModelInfo) string {
	var parts []string
	if m.SWEScore > 0 {
		parts = append(parts, fmt.Sprintf("SWE %.0f%%", m.SWEScore))
	}
	if m.LCBScore > 0 {
		parts = append(parts, fmt.Sprintf("LCB %.0f%%", m.LCBScore))
	}
	if m.IOIScore > 0 {
		parts = append(parts, fmt.Sprintf("IOI %.0f%%", m.IOIScore))
	}
	if m.ContextWindow > 0 {
		ctx := m.ContextWindow
		switch {
		case ctx >= 1_000_000:
			parts = append(parts, fmt.Sprintf("%dM ctx", ctx/1_000_000))
		case ctx >= 1_000:
			parts = append(parts, fmt.Sprintf("%dk ctx", ctx/1_000))
		default:
			parts = append(parts, fmt.Sprintf("%d ctx", ctx))
		}
	}
	if m.PricingInput > 0 {
		parts = append(parts, fmt.Sprintf("$%.2f/M in", m.PricingInput))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, " · ")
}

// isLiteLLMRunning probes localhost:4000 to check if a LiteLLM proxy is up.
func isLiteLLMRunning() bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get("http://localhost:4000/health")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

func stripANSI(s string) string {
	var out strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '\033' && i+1 < len(s) && s[i+1] == '[' {
			i += 2
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		out.WriteByte(s[i])
		i++
	}
	return out.String()
}

func bold(s string) string { return "\033[1m" + s + "\033[0m" }
func dim(s string) string  { return "\033[2m" + s + "\033[0m" }
