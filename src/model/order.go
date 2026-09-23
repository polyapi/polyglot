package model

type schemaRef struct {
	Path            string
	PublicNamespace string
}

// OrderSchemas topologically sorts schemas so x-poly-ref dependencies train first.
// Cycles (and unknown refs) keep original relative order, matching the TS CLI.
func OrderSchemas(schemas []map[string]any) []map[string]any {
	if len(schemas) <= 1 {
		return schemas
	}

	byName := make(map[string]map[string]any, len(schemas))
	original := make([]string, 0, len(schemas))
	for _, schema := range schemas {
		name := schemaContextName(schema)
		byName[name] = schema
		original = append(original, name)
	}

	adjacency := make(map[string][]string, len(original))
	seenEdge := make(map[string]map[string]struct{}, len(original))
	indegree := make(map[string]int, len(original))
	for _, name := range original {
		adjacency[name] = nil
		seenEdge[name] = map[string]struct{}{}
		indegree[name] = 0
	}

	for _, schema := range schemas {
		name := schemaContextName(schema)
		for _, ref := range polySchemaRefs(schema["definition"]) {
			if ref.PublicNamespace != "" || ref.Path == "" {
				continue
			}
			if _, ok := byName[ref.Path]; !ok {
				continue
			}
			if _, seen := seenEdge[ref.Path][name]; seen {
				continue
			}
			seenEdge[ref.Path][name] = struct{}{}
			adjacency[ref.Path] = append(adjacency[ref.Path], name)
			indegree[name]++
		}
	}

	queue := make([]string, 0, len(original))
	for _, name := range original {
		if indegree[name] == 0 {
			queue = append(queue, name)
		}
	}

	ordered := make([]string, 0, len(original))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		ordered = append(ordered, current)
		for _, dep := range adjacency[current] {
			indegree[dep]--
			if indegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(ordered) != len(schemas) {
		seen := make(map[string]struct{}, len(ordered))
		for _, name := range ordered {
			seen[name] = struct{}{}
		}
		for _, name := range original {
			if _, ok := seen[name]; !ok {
				ordered = append(ordered, name)
			}
		}
	}

	out := make([]map[string]any, 0, len(ordered))
	for _, name := range ordered {
		if schema, ok := byName[name]; ok {
			out = append(out, schema)
		}
	}
	return out
}

func polySchemaRefs(node any) []schemaRef {
	switch n := node.(type) {
	case []any:
		var out []schemaRef
		for _, item := range n {
			out = append(out, polySchemaRefs(item)...)
		}
		return out
	case map[string]any:
		if raw, ok := n["x-poly-ref"]; ok {
			if ref, ok := asMap(raw); ok {
				path, _ := ref["path"].(string)
				if path != "" {
					ns, _ := ref["publicNamespace"].(string)
					return []schemaRef{{Path: path, PublicNamespace: ns}}
				}
			}
		}
		var out []schemaRef
		for _, key := range []string{"schemas", "properties", "patternProperties"} {
			if nested, ok := asMap(n[key]); ok {
				for _, v := range nested {
					out = append(out, polySchemaRefs(v)...)
				}
			}
		}
		if items, ok := n["items"]; ok {
			out = append(out, polySchemaRefs(items)...)
		}
		for _, key := range []string{"additionalItems", "additionalProperties"} {
			if v, ok := n[key]; ok && !isJSONBool(v) && v != nil {
				out = append(out, polySchemaRefs(v)...)
			}
		}
		for _, key := range []string{"allOf", "anyOf", "oneOf"} {
			out = append(out, polySchemaRefs(n[key])...)
		}
		return out
	default:
		return nil
	}
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func isJSONBool(v any) bool {
	_, ok := v.(bool)
	return ok
}
