package llm

import (
	"context"
	"fmt"
	"strings"
)

// Classify returns the category from categories that best fits text — the
// canonical tier-1 capability helper (docs/adr/0012-internal-services-two-tier.md):
// pure composition over CompleteJSON, with the model's answer validated
// against the caller's list so apps never receive an invented category.
func (c *Client) Classify(ctx context.Context, text string, categories []string) (string, error) {
	if len(categories) < 2 {
		return "", fmt.Errorf("classify: need at least 2 categories")
	}
	prompt := fmt.Sprintf(
		"Classify the following text into exactly one of these categories: %s.\nRespond as {\"category\": \"<one of the categories, verbatim>\"}.\n\nText:\n%s",
		strings.Join(categories, ", "), text)

	var out struct {
		Category string `json:"category"`
	}
	if err := c.CompleteJSON(ctx, prompt, &out); err != nil {
		return "", err
	}
	for _, cat := range categories {
		if strings.EqualFold(strings.TrimSpace(out.Category), cat) {
			return cat, nil
		}
	}
	return "", fmt.Errorf("classify: model returned %q, not one of the given categories", out.Category)
}
