package memory

import chromem "github.com/philippgille/chromem-go"

// NewOpenAICompatEmbedFn returns an EmbeddingFunc backed by any OpenAI-compatible
// embeddings endpoint (OpenAI, Ollama, Mistral, etc.).
// Anthropic does not expose an OpenAI-compatible embeddings endpoint — pass nil
// to memory.New when using Anthropic; semantic search will be skipped.
func NewOpenAICompatEmbedFn(baseURL, apiKey, model string) chromem.EmbeddingFunc {
	return chromem.NewEmbeddingFuncOpenAICompat(baseURL, apiKey, model, nil)
}

// NewEmbedFnFromConfig selects an embedding function based on the configured LLM provider.
// For Anthropic it returns nil (no native embedding endpoint).
// For all other providers it uses the OpenAI-compat endpoint at the configured BaseURL.
func NewEmbedFnFromConfig(provider, baseURL, apiKey string) chromem.EmbeddingFunc {
	if provider == "anthropic" {
		return nil
	}
	return NewOpenAICompatEmbedFn(baseURL, apiKey, "text-embedding-3-small")
}
