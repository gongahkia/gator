package model

import (
	"sort"
	"strings"
)

// openCodeCatalogVersion is the upstream documentation revision used for this
// checked-in snapshot. Refreshing it is an intentional source change; model
// selection must not depend on a runtime catalog network request.
const openCodeCatalogVersion = "2026-08-17"

type openCodeProtocol uint8

const (
	openCodeChatCompletions openCodeProtocol = iota
	openCodeResponses
	openCodeAnthropic
	openCodeGemini
)

type openCodeCatalogGroup struct {
	protocol openCodeProtocol
	models   []string
}

// openCodeCatalog is derived from https://opencode.ai/docs/zen and
// https://opencode.ai/docs/go on 2026-08-17. The provider publishes one model
// table per gateway, and individual entries—not model name families—select the
// protocol.
var openCodeCatalog = map[Provider]map[string]openCodeProtocol{
	OpenCode: catalog(
		openCodeCatalogGroup{openCodeResponses, []string{
			"gpt-5.6-sol", "gpt-5.6-terra", "gpt-5.6-luna", "gpt-5.5", "gpt-5.5-pro",
			"gpt-5.4", "gpt-5.4-pro", "gpt-5.4-mini", "gpt-5.4-nano", "gpt-5.3-codex",
			"gpt-5.3-codex-spark", "gpt-5.2", "gpt-5.2-codex", "gpt-5.1", "gpt-5.1-codex",
			"gpt-5.1-codex-max", "gpt-5.1-codex-mini", "gpt-5", "gpt-5-codex", "gpt-5-nano",
			"grok-4.6", "grok-4.5", "grok-build-0.1", "muse-spark-1.2",
		}},
		openCodeCatalogGroup{openCodeAnthropic, []string{
			"claude-fable-5", "claude-opus-5", "claude-opus-4-8", "claude-opus-4-7", "claude-opus-4-6",
			"claude-opus-4-5", "claude-sonnet-5", "claude-sonnet-4-6", "claude-sonnet-4-5", "claude-haiku-4-5",
			"qwen3.7-max", "qwen3.7-plus", "qwen3.6-plus", "qwen3.5-plus",
		}},
		openCodeCatalogGroup{openCodeGemini, []string{
			"gemini-3.7-flash", "gemini-3.6-flash", "gemini-3.5-flash", "gemini-3.5-flash-lite", "gemini-3.1-pro", "gemini-3-flash",
		}},
		openCodeCatalogGroup{openCodeChatCompletions, []string{
			"deepseek-v4-pro", "deepseek-v4-flash", "minimax-m3", "minimax-m2.7", "minimax-m2.5", "glm-5.2", "glm-5.1", "glm-5",
			"kimi-k2.5", "kimi-k2.6", "kimi-k2.7-code", "kimi-k3", "big-pickle", "mimo-v2.5-free", "hy3-free", "laguna-s-2.1-free",
			"nemotron-3-ultra-free", "nemotron-3.5-lightning-free", "deepseek-v4-flash-free",
		}},
	),
	OpenCodeGo: catalog(
		openCodeCatalogGroup{openCodeResponses, []string{"grok-4.5", "gpt-5.6-luna"}},
		openCodeCatalogGroup{openCodeAnthropic, []string{
			"minimax-m3", "minimax-m2.7", "minimax-m2.5", "qwen3.8-max", "qwen3.7-max", "qwen3.7-plus", "qwen3.6-plus",
		}},
		openCodeCatalogGroup{openCodeChatCompletions, []string{
			"glm-5.3", "glm-5.2", "glm-5.1", "kimi-k3", "kimi-k2.7-code", "kimi-k2.6", "deepseek-v4-pro", "deepseek-v4-flash",
			"mimo-v2.5", "mimo-v2.5-pro", "hy3",
		}},
	),
}

func catalog(groups ...openCodeCatalogGroup) map[string]openCodeProtocol {
	entries := make(map[string]openCodeProtocol)
	for _, group := range groups {
		for _, model := range group.models {
			if _, exists := entries[model]; exists {
				panic("duplicate OpenCode catalog model " + model)
			}
			entries[model] = group.protocol
		}
	}
	return entries
}

func openCodeModelProtocol(provider Provider, model string) (openCodeProtocol, bool) {
	entries, ok := openCodeCatalog[provider]
	if !ok {
		return 0, false
	}
	protocol, ok := entries[strings.TrimSpace(model)]
	return protocol, ok
}

// CuratedModels returns the checked-in, selectable model IDs for a provider.
// For OpenCode gateways the list is the complete embedded upstream catalog;
// for other providers it contains only Gator's stable default. Callers must
// still allow an explicit custom model ID for account-specific deployments.
func CuratedModels(provider Provider) []string {
	defaultModel := DefaultModel(provider)
	entries, found := openCodeCatalog[provider]
	if !found {
		if defaultModel == "" {
			return nil
		}
		return []string{defaultModel}
	}

	models := make([]string, 0, len(entries))
	for model := range entries {
		if model != defaultModel {
			models = append(models, model)
		}
	}
	sort.Strings(models)
	if defaultModel != "" {
		return append([]string{defaultModel}, models...)
	}
	return models
}

func openCodeProviderName(provider Provider) string {
	if provider == OpenCodeGo {
		return "OpenCode Go"
	}
	return "OpenCode Zen"
}
