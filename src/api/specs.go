package api

import (
	"strings"
	"time"
)

const specsTimeout = 2 * time.Minute

// Specs is GET /specs with the same filters as the language SDKs.
// The body is an opaque JSON array; the host does not interpret specification unions.
func (c *HTTPClient) Specs(contexts, names, ids []string, noTypes bool) ([]any, error) {
	if c == nil {
		return nil, statusErr("GET", "specs", 0, "no HTTP client")
	}
	items, _, err := c.WithTimeout(specsTimeout).listRaw("specs", specsQuery(contexts, names, ids, noTypes))
	return items, err
}

func specsQuery(contexts, names, ids []string, noTypes bool) [][2]string {
	var q [][2]string
	if len(contexts) > 0 {
		q = append(q, [2]string{"contexts", strings.Join(contexts, ",")})
	}
	if len(names) > 0 {
		q = append(q, [2]string{"names", strings.Join(names, ",")})
	}
	if len(ids) > 0 {
		q = append(q, [2]string{"ids", strings.Join(ids, ",")})
	}
	if noTypes {
		q = append(q, [2]string{"noTypes", "true"})
	}
	return q
}

// DescribeCustomFunction POSTs to /functions/{server|client}/description-generation
// or /webhooks/description-generation. kind is server, client, webhook, or the
// Glide type names (server-function, …).
func (c *HTTPClient) DescribeCustomFunction(kind string, payload any) (map[string]any, error) {
	if c == nil {
		return nil, statusErr("POST", "description-generation", 0, "no HTTP client")
	}
	value, err := c.PostQuery(descriptionResource(kind), nil, payload)
	if err != nil {
		return nil, err
	}
	if m, ok := value.(map[string]any); ok {
		return m, nil
	}
	return map[string]any{}, nil
}

func descriptionResource(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "client", "client-function":
		return "functions/client/description-generation"
	case "webhook":
		return "webhooks/description-generation"
	default:
		return "functions/server/description-generation"
	}
}
