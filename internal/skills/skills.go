package skills

import (
	_ "embed"
)

//go:embed agenthub-spec/SKILL.md
var AgentHubSpec string

func Files() map[string]string {
	return map[string]string{
		"agenthub-spec/SKILL.md": AgentHubSpec,
	}
}
