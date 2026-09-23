package model

import (
	"fmt"

	"github.com/polyapi/polyglot/src/api"
)

// TrainResult is upsert counts plus per-resource failures.
type TrainResult struct {
	Functions int
	Webhooks  int
	Schemas   int
	Created   []TrainedResource
	Failed    []FailedResource
}

// TrainedResource is one successfully upserted function, webhook, or schema.
type TrainedResource struct {
	Kind    string
	ID      string
	Name    string
	Context string
}

// FailedResource is one upsert that did not succeed.
type FailedResource struct {
	Kind    string
	Index   int
	Name    string
	Context string
	Reason  string
}

// CreatedCount is successful upserts.
func (r TrainResult) CreatedCount() int {
	return r.Functions + r.Webhooks + r.Schemas
}

// TrainingRequirements are permission names needed for this spec (TS buildModelTrainingRequirements).
func TrainingRequirements(spec SpecInput) []api.PermissionReq {
	var reqs []api.PermissionReq
	if len(spec.Functions) > 0 {
		reqs = append(reqs, api.Requirement("manageApiFunctions"))
	}
	if len(spec.Schemas) > 0 {
		reqs = append(reqs, api.Requirement("manageSchemas"))
	}
	if len(spec.Webhooks) > 0 {
		reqs = append(reqs, api.Requirement("manageWebhooks"))
	}
	return reqs
}

// Train upserts API functions, webhook handles, then schemas (dependency-ordered).
func Train(client *api.HTTPClient, path string, onProgress func(string)) (TrainResult, error) {
	spec, err := readSpecFile(path)
	if err != nil {
		return TrainResult{}, err
	}
	return TrainSpec(client, spec, onProgress)
}

// TrainSpec upserts a parsed specification input.
func TrainSpec(client *api.HTTPClient, spec SpecInput, onProgress func(string)) (TrainResult, error) {
	if client == nil {
		return TrainResult{}, fmt.Errorf("missing API client")
	}
	if len(spec.Functions) == 0 && len(spec.Webhooks) == 0 && len(spec.Schemas) == 0 {
		return TrainResult{}, fmt.Errorf("specification input has no functions, webhooks, or schemas")
	}

	var out TrainResult
	fn := executeTraining(client, spec.Functions, "api function", "functions/api", onProgress, true)
	out.Functions = countKind(fn.created, "api function")
	out.Created = append(out.Created, fn.created...)
	out.Failed = append(out.Failed, fn.failed...)

	wh := executeTraining(client, spec.Webhooks, "webhook", "webhooks", onProgress, true)
	out.Webhooks = countKind(wh.created, "webhook")
	out.Created = append(out.Created, wh.created...)
	out.Failed = append(out.Failed, wh.failed...)

	// Schemas must go out in dependency order; concurrent PUTs would race the platform.
	sc := executeTraining(client, OrderSchemas(spec.Schemas), "schema", "schemas", onProgress, false)
	out.Schemas = countKind(sc.created, "schema")
	out.Created = append(out.Created, sc.created...)
	out.Failed = append(out.Failed, sc.failed...)
	return out, nil
}

type trainBatch struct {
	created []TrainedResource
	failed  []FailedResource
}

func executeTraining(client *api.HTTPClient, resources []map[string]any, kind, collection string, onProgress func(string), parallel bool) trainBatch {
	if len(resources) == 0 {
		return trainBatch{}
	}
	results := make([]settled[any], len(resources))
	forChunks(len(resources), func(from, to int) {
		if onProgress != nil {
			onProgress(fmt.Sprintf("Training from %s number %d to %s number %d out of %d", kind, from+1, kind, to, len(resources)))
		}
		if !parallel {
			for i := from; i < to; i++ {
				v, err := client.Put(collection, resources[i])
				results[i] = settled[any]{value: v, err: err}
			}
			return
		}
		chunk := runSettled(to-from, func(i int) (any, error) {
			return client.Put(collection, resources[from+i])
		})
		copy(results[from:to], chunk)
	})

	var batch trainBatch
	for i, result := range results {
		resource := resources[i]
		if result.err != nil {
			batch.failed = append(batch.failed, FailedResource{
				Kind:    kind,
				Index:   i,
				Name:    mapString(resource, "name"),
				Context: mapString(resource, "context"),
				Reason:  describeError(result.err),
			})
			continue
		}
		id, name, ctx := trainedIdentity(result.value, resource)
		batch.created = append(batch.created, TrainedResource{
			Kind:    kind,
			ID:      id,
			Name:    name,
			Context: ctx,
		})
	}
	return batch
}

func trainedIdentity(value any, fallback map[string]any) (id, name, ctx string) {
	if m, ok := asMap(value); ok {
		id = mapString(m, "id")
		name = mapString(m, "name")
		ctx = mapString(m, "context")
	}
	if name == "" {
		name = mapString(fallback, "name")
	}
	if ctx == "" {
		ctx = mapString(fallback, "context")
	}
	return id, name, ctx
}

func countKind(created []TrainedResource, kind string) int {
	n := 0
	for _, c := range created {
		if c.Kind == kind {
			n++
		}
	}
	return n
}

func (r TrainedResource) DisplayName() string {
	if r.Context == "" {
		return r.Name
	}
	return r.Context + "." + r.Name
}
