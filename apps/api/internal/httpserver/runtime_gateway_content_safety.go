package httpserver

import (
	admincontrolcontract "github.com/srapi/srapi/apps/api/internal/modules/admin_control/contract"
	contentsafetycontract "github.com/srapi/srapi/apps/api/internal/modules/content_safety/contract"
)

func contentSafetyConfigFromAdminControl(config admincontrolcontract.ContentSafetyConfig) contentsafetycontract.Config {
	return contentsafetycontract.Config{
		Enabled:              config.Enabled,
		Mode:                 contentsafetycontract.Mode(config.Mode),
		RedactPII:            config.RedactPII,
		BlockPII:             config.BlockPII,
		BlockPromptInjection: config.BlockPromptInjection,
		BlockCustomKeywords:  config.BlockCustomKeywords,
		CustomKeywords:       append([]string(nil), config.CustomKeywords...),
		ModelScopes:          append([]string(nil), config.ModelScopes...),
	}
}

func contentSafetyFindingsAudit(findings []contentsafetycontract.Finding) []map[string]any {
	out := make([]map[string]any, 0, len(findings))
	for _, finding := range findings {
		out = append(out, map[string]any{
			"kind":     string(finding.Kind),
			"severity": string(finding.Severity),
			"count":    finding.Count,
			"redacted": finding.Redacted,
		})
	}
	return out
}
