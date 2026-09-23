package api

import "encoding/json"

// UnwrapList turns a list endpoint body into items.
// Accepts a JSON array, or an object with data / items / results / content.
func UnwrapList(url string, value any) ([]any, error) {
	switch v := value.(type) {
	case []any:
		return v, nil
	case map[string]any:
		for _, key := range []string{"data", "items", "results", "content"} {
			if arr, ok := v[key].([]any); ok {
				return arr, nil
			}
		}
		return nil, unexpectedList(url)
	default:
		return nil, unexpectedList(url)
	}
}

// NextPage returns the next page hint if the payload looks paginated.
// Top-level page/totalPages is 0-indexed (functions). Nested pagination.page/pages
// (GET /tables) is 1-indexed.
func NextPage(value any) (uint64, bool) {
	obj, ok := value.(map[string]any)
	if !ok {
		return 0, false
	}
	if next, ok := nextPageFrom(obj, false); ok {
		return next, true
	}
	if pag, ok := obj["pagination"].(map[string]any); ok {
		return nextPageFrom(pag, true)
	}
	return 0, false
}

func nextPageFrom(obj map[string]any, oneIndexed bool) (uint64, bool) {
	page, ok := asUint64(obj["page"])
	if !ok {
		return 0, false
	}
	totalKeys := []string{"totalPages", "total_pages"}
	if oneIndexed {
		totalKeys = []string{"pages", "totalPages", "total_pages"}
	}
	if total, ok := asUint64(first(obj, totalKeys...)); ok {
		if oneIndexed {
			if page < total {
				return page + 1, true
			}
		} else if page+1 < total {
			return page + 1, true
		}
	}
	if obj["hasMore"] == true || obj["has_more"] == true {
		return page + 1, true
	}
	return 0, false
}

func first(obj map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := obj[k]; ok {
			return v
		}
	}
	return nil
}

func asUint64(v any) (uint64, bool) {
	switch n := v.(type) {
	case float64:
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	case json.Number:
		i, err := n.Int64()
		if err != nil || i < 0 {
			return 0, false
		}
		return uint64(i), true
	case int:
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	case int64:
		if n < 0 {
			return 0, false
		}
		return uint64(n), true
	default:
		return 0, false
	}
}
