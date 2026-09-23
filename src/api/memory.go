package api

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// MemoryClient is an in-memory Client for unit tests.
type MemoryClient struct {
	mu    sync.Mutex
	store map[string]map[string]any
	seq   atomic.Uint64
}

func NewMemoryClient() *MemoryClient {
	return &MemoryClient{store: map[string]map[string]any{}}
}

func (c *MemoryClient) Insert(resource, id string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.store[resource] == nil {
		c.store[resource] = map[string]any{}
	}
	c.store[resource][id] = value
}

func (c *MemoryClient) List(resource string) ([]any, error) {
	if err := checkResource(resource); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := c.store[resource]
	out := make([]any, 0, len(rows))
	for _, v := range rows {
		out = append(out, v)
	}
	return out, nil
}

func (c *MemoryClient) ListAll(resource string) ([]any, error) {
	return c.List(resource)
}

func (c *MemoryClient) Get(resource, id string) (any, error) {
	if err := checkResource(resource); err != nil {
		return nil, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	row, ok := c.store[resource][id]
	if !ok {
		return nil, FromStatus("GET", resource, 404, "not found")
	}
	return row, nil
}

func (c *MemoryClient) Create(resource string, payload any) (any, error) {
	if err := checkResource(resource); err != nil {
		return nil, err
	}
	row := cloneMap(payload)
	id := ""
	if m, ok := row.(map[string]any); ok {
		if existing, ok := m["id"].(string); ok && existing != "" {
			id = existing
		}
	}
	if id == "" {
		id = fmt.Sprintf("mem-%d", c.seq.Add(1)-1)
	}
	if m, ok := row.(map[string]any); ok {
		m["id"] = id
	}
	c.Insert(resource, id, row)
	return row, nil
}

func (c *MemoryClient) Update(resource, id string, payload any, _ map[string]string) (any, error) {
	existing, err := c.Get(resource, id)
	if err != nil {
		return nil, err
	}
	dest, destOK := existing.(map[string]any)
	src, srcOK := payload.(map[string]any)
	var row any
	if destOK && srcOK {
		merged := map[string]any{}
		for k, v := range dest {
			merged[k] = v
		}
		for k, v := range src {
			merged[k] = v
		}
		merged["id"] = id
		row = merged
	} else {
		row = payload
	}
	c.Insert(resource, id, row)
	return row, nil
}

func (c *MemoryClient) Delete(resource, id string, _ map[string]string) error {
	if err := checkResource(resource); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	rows := c.store[resource]
	if rows == nil {
		return FromStatus("DELETE", resource, 404, "not found")
	}
	if _, ok := rows[id]; !ok {
		return FromStatus("DELETE", resource, 404, "not found")
	}
	delete(rows, id)
	return nil
}

func checkResource(resource string) error {
	if IsSafeResource(resource) {
		return nil
	}
	return invalidResource(resource)
}

func cloneMap(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(m))
	for k, val := range m {
		out[k] = val
	}
	return out
}
