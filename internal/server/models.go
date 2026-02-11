package server

type orModels struct {
	Data []struct {
		ID                  string   `json:"id"`
		ContextLength       int      `json:"context_length"`
		SupportedParameters []string `json:"supported_parameters"`
		TopProvider         struct {
			ContextLength int `json:"context_length"`
		} `json:"top_provider"`
		Pricing struct {
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
		} `json:"pricing"`
	} `json:"data"`
}

func supportsToolUse(supportedParams []string) bool {
	for _, param := range supportedParams {
		if param == "tools" || param == "tool_choice" {
			return true
		}
	}
	return false
}
