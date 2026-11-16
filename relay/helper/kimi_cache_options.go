package helper

import (
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
)

var (
	kimiPromptCacheModes = []string{"implicit", "explicit"}
	kimiCacheTTLs        = []string{"5m", "1h"}
	kimiCacheTypes       = []string{"ephemeral"}
)

// validateKimiCacheWriteOptions enforces Kimi's documented cache-write
// contract for kimi-* models: Chat/Responses `prompt_cache_options`
// (mode, ttl) and Messages top-level `cache_control` (type, ttl). Some
// Kimi-compatible upstreams silently accept invalid values (an unknown ttl
// falls back to 5m), so the gateway rejects them with a 400 naming the
// offending field, as the official API does. Absent options are untouched.
func validateKimiCacheWriteOptions(model string, field string, raw json.RawMessage, enumFields map[string][]string) error {
	if !strings.HasPrefix(strings.ToLower(model), "kimi-") {
		return nil
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	var options map[string]json.RawMessage
	if err := common.Unmarshal(raw, &options); err != nil || options == nil {
		return invalidKimiCacheOptionError(field, fmt.Sprintf("%s must be an object", field))
	}
	for key, allowed := range enumFields {
		value, ok := options[key]
		if !ok {
			continue
		}
		param := field + "." + key
		var s string
		if err := common.Unmarshal(value, &s); err != nil {
			return invalidKimiCacheOptionError(param, fmt.Sprintf("invalid %s: must be one of %s", param, strings.Join(allowed, ", ")))
		}
		if !slices.Contains(allowed, s) {
			return invalidKimiCacheOptionError(param, fmt.Sprintf("invalid %s %q: must be one of %s", param, s, strings.Join(allowed, ", ")))
		}
	}
	return nil
}

func invalidKimiCacheOptionError(param string, message string) error {
	return types.WithOpenAIError(types.OpenAIError{
		Message: message,
		Type:    "invalid_request_error",
		Param:   param,
		Code:    "invalid_request",
	}, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}
