package advisor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"dep-health/models"
)

const advisorSystemPrompt = `You are a software dependency security advisor.
You will receive structured information about a dependency that needs upgrading.
Call the record_advisory tool with your analysis. Be concise and practical.
Focus on what engineers need to do, not on general advice.`

// AnthropicAdvisor calls the Anthropic Messages API to generate rich,
// context-aware upgrade summaries and migration steps.
//
// Activate by setting ANTHROPIC_API_KEY — the pipeline selects this
// implementation automatically when the key is present.  Without the key,
// pipeline.Run falls back to StubAdvisor so nothing breaks during development.
type AnthropicAdvisor struct {
	client anthropic.Client
}

// NewAnthropic creates an AnthropicAdvisor. Returns an error if apiKey is empty.
func NewAnthropic(apiKey string) (*AnthropicAdvisor, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("advisor: ANTHROPIC_API_KEY must not be empty")
	}
	return &AnthropicAdvisor{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
	}, nil
}

// advisoryOutput matches the tool input schema defined in Advise.
type advisoryOutput struct {
	Summary         string   `json:"summary"`
	BreakingChanges []string `json:"breaking_changes"`
	MigrationSteps  []string `json:"migration_steps"`
}

// Advise calls the Anthropic API using tool use to produce a structured
// AdvisoryReport.  Falls back to StubAdvisor on any API or parse error so
// the pipeline always produces output even when the key is set but the API
// is temporarily unavailable.
func (a *AnthropicAdvisor) Advise(ctx context.Context, dep models.ScoredDependency) (models.AdvisoryReport, error) {
	// The "record_advisory" tool forces the model to return structured JSON
	// rather than free-form prose, making parsing reliable.
	tool := anthropic.ToolParam{
		Name:        "record_advisory",
		Description: anthropic.String("Record the structured advisory report for this dependency upgrade"),
		InputSchema: anthropic.ToolInputSchemaParam{
			Properties: map[string]any{
				"summary": map[string]any{
					"type":        "string",
					"description": "2-3 sentence summary: what changed, why it matters, and upgrade urgency",
				},
				"breaking_changes": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Specific breaking API changes engineers must handle. Empty array if none.",
				},
				"migration_steps": map[string]any{
					"type":        "array",
					"items":       map[string]any{"type": "string"},
					"description": "Concrete, numbered steps to perform the upgrade safely",
				},
			},
		},
	}

	msg, err := a.client.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.ModelClaudeOpus4_6,
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: advisorSystemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(buildAnthropicPrompt(dep))),
		},
		Tools:      []anthropic.ToolUnionParam{{OfTool: &tool}},
		ToolChoice: anthropic.ToolChoiceParamOfTool("record_advisory"),
	})
	if err != nil {
		// API unavailable or rate-limited — degrade gracefully.
		return NewStub().Advise(ctx, dep)
	}

	// Extract the tool call input from the response content.
	var out advisoryOutput
	for _, block := range msg.Content {
		if variant, ok := block.AsAny().(anthropic.ToolUseBlock); ok {
			raw := variant.JSON.Input.Raw()
			if jsonErr := json.Unmarshal([]byte(raw), &out); jsonErr != nil {
				return NewStub().Advise(ctx, dep)
			}
			break
		}
	}

	// If the model somehow produced no tool call, fall back to stub.
	if out.Summary == "" {
		return NewStub().Advise(ctx, dep)
	}

	return models.AdvisoryReport{
		ScoredDependency: dep,
		Summary:          out.Summary,
		BreakingChanges:  out.BreakingChanges,
		MigrationSteps:   out.MigrationSteps,
	}, nil
}

// buildAnthropicPrompt constructs the user message sent to the model.
func buildAnthropicPrompt(dep models.ScoredDependency) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Package:    %s (%s)\n", dep.Name, dep.Ecosystem)
	fmt.Fprintf(&b, "Upgrade:    %s → %s (%s version bump, %d releases behind)\n",
		dep.CurrentVersion, dep.LatestVersion, dep.SeverityGap, dep.VersionsBehind)
	fmt.Fprintf(&b, "Risk score: %.1f / 100\n", dep.RiskScore)

	if len(dep.Vulnerabilities) > 0 {
		fmt.Fprintf(&b, "\nKnown vulnerabilities (%d):\n", len(dep.Vulnerabilities))
		for _, v := range dep.Vulnerabilities {
			fmt.Fprintf(&b, "  • %s [%s]: %s\n", v.ID, v.Severity, v.Summary)
		}
	}

	if len(dep.Reasons) > 0 {
		fmt.Fprintf(&b, "\nRisk factors: %s\n", strings.Join(dep.Reasons, "; "))
	}

	if dep.CascadeGroup != "" {
		members := strings.ReplaceAll(dep.CascadeGroup, "+", ", ")
		fmt.Fprintf(&b, "\nCascade group: %s (must all be upgraded together)\n", members)
	}

	if len(dep.BlockedBy) > 0 {
		fmt.Fprintf(&b, "Blocked by: %s (peer has no compatible release yet)\n",
			strings.Join(dep.BlockedBy, ", "))
	}

	return b.String()
}
