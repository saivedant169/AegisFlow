package router

// ModelAlias maps friendly names to actual model identifiers.
// Example: "fast" -> "gpt-4o-mini", "smart" -> "gpt-4o", "local" -> "qwen2.5:0.5b"
type ModelAlias struct {
	aliases map[string]string
}

func NewModelAlias(aliases map[string]string) *ModelAlias {
	if aliases == nil {
		aliases = make(map[string]string)
	}
	return &ModelAlias{aliases: aliases}
}

// Resolve returns the actual model name for an alias.
// If no alias exists, returns the original name unchanged.
func (a *ModelAlias) Resolve(model string) string {
	if actual, ok := a.aliases[model]; ok {
		return actual
	}
	return model
}

// List returns all configured aliases.
func (a *ModelAlias) List() map[string]string {
	result := make(map[string]string, len(a.aliases))
	for k, v := range a.aliases {
		result[k] = v
	}
	return result
}
