package service

import (
	_ "embed"
	"strings"
)

// Real Codex CLI base instructions (the `instructions` system field the upstream
// ChatGPT/Codex backend validates per model). Ported verbatim from the sub2api
// reference gateway so requests that arrive without a Codex CLI base prompt are
// still accepted upstream — sending an arbitrary placeholder gets the request
// rejected, which is what broke gpt-5.5 and the other gpt-5.x Codex models.
//
//go:embed codex_instructions.txt
var codexBaseInstructions string

//go:embed codex_instructions_gpt5_1.txt
var codexInstructionsGPT51 string

//go:embed codex_instructions_gpt5_2.txt
var codexInstructionsGPT52 string

// codexBaseInstructionsForModel returns the real Codex CLI base prompt that
// matches the model, mirroring sub2api's CodexBaseInstructionsForModel:
//
//	any "*codex*"        -> GPT-5-Codex base prompt
//	gpt-5.2*             -> GPT-5.2 prompt
//	gpt-5.1* / gpt-5*    -> GPT-5.1 prompt (this is the gpt-5.5 path)
//	otherwise            -> GPT-5-Codex base prompt (safe default)
func codexBaseInstructionsForModel(model string) string {
	m := strings.ToLower(strings.TrimSpace(model))
	switch {
	case strings.Contains(m, "codex"):
		return codexBaseInstructions
	case strings.HasPrefix(m, "gpt-5.2"):
		if v := strings.TrimSpace(codexInstructionsGPT52); v != "" {
			return codexInstructionsGPT52
		}
	case strings.HasPrefix(m, "gpt-5.1"), strings.HasPrefix(m, "gpt-5"):
		if v := strings.TrimSpace(codexInstructionsGPT51); v != "" {
			return codexInstructionsGPT51
		}
	}
	return codexBaseInstructions
}
