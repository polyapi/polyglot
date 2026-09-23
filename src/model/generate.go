package model

import (
	"errors"
	"fmt"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/polyapi/polyglot/src/api"
)

const generateHTTPTimeout = 2 * time.Minute

// GenerateOptions are flags for `polyapi model generate`.
type GenerateOptions struct {
	Context           string
	HostURL           string
	HostURLAsArgument string
	DisableAI         bool
	Rename            []Rename
	SourcePath        string
	Destination       string
	OnProgress        func(string)
}

// GenerateResult is the written specification input plus AI warnings.
type GenerateResult struct {
	Path     string
	Warnings []string
}

func (o GenerateOptions) progress(msg string) {
	if o.OnProgress != nil {
		o.OnProgress(msg)
	}
}

// Generate translates an OpenAPI document into a specification input file.
func Generate(client *api.HTTPClient, opts GenerateOptions) (GenerateResult, error) {
	if opts.HostURL != "" && !isValidHTTPURL(opts.HostURL) {
		return GenerateResult{}, fmt.Errorf("%s is not a valid url", opts.HostURL)
	}

	contents, err := readSpecSource(opts.SourcePath)
	if err != nil {
		return GenerateResult{}, err
	}

	if opts.Destination != "" {
		if _, err := resolveOutputPath(opts.SourcePath, opts.Destination); err != nil {
			return GenerateResult{}, err
		}
	}

	if client != nil {
		client = client.WithTimeout(generateHTTPTimeout)
	}

	spec, err := translateOAS(client, contents, opts.Context, opts.HostURL, opts.HostURLAsArgument)
	if err != nil {
		return GenerateResult{}, err
	}

	var warnings []string
	fnWarn, err := describeFunctions(client, spec.Functions, opts)
	if err != nil {
		return GenerateResult{}, err
	}
	warnings = append(warnings, fnWarn...)

	whWarn, err := describeWebhooks(client, spec.Webhooks, opts)
	if err != nil {
		return GenerateResult{}, err
	}
	warnings = append(warnings, whWarn...)

	outPath, err := finalOutputPath(opts, spec.Title)
	if err != nil {
		return GenerateResult{}, err
	}

	raw, err := Encode(spec)
	if err != nil {
		return GenerateResult{}, err
	}
	text := ApplyRename(string(raw)+"\n", opts.Rename)
	if err := writeFile(outPath, text); err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{Path: outPath, Warnings: warnings}, nil
}

func finalOutputPath(opts GenerateOptions, title string) (string, error) {
	if opts.Destination != "" {
		return resolveOutputPath(opts.SourcePath, opts.Destination)
	}
	return uniqueOutputPath(outputBaseDir(opts.SourcePath), title)
}

func translateOAS(client *api.HTTPClient, contents, context, hostURL, hostURLAsArgument string) (SpecInput, error) {
	if client == nil {
		return SpecInput{}, fmt.Errorf("missing API client")
	}
	var query [][2]string
	if context != "" {
		query = append(query, [2]string{"context", context})
	}
	if hostURL != "" {
		query = append(query, [2]string{"hostUrl", hostURL})
	}
	if hostURLAsArgument != "" {
		query = append(query, [2]string{"hostUrlAsArgument", hostURLAsArgument})
	}
	value, err := client.PostText("specification-input/oas", query, contents)
	if err != nil {
		return SpecInput{}, err
	}
	return decodeSpec(value)
}

func describeFunctions(client *api.HTTPClient, resources []map[string]any, opts GenerateOptions) ([]string, error) {
	return processResources(resources, "function", opts, func(resource map[string]any) (map[string]any, error) {
		if opts.DisableAI {
			return resource, nil
		}
		payload := map[string]any{
			"name":        resource["name"],
			"context":     resource["context"],
			"description": resource["description"],
			"arguments":   resource["arguments"],
			"source":      resource["source"],
		}
		value, err := client.PostQuery("functions/api/description-generation", nil, payload)
		if err != nil {
			return nil, err
		}
		ai, _ := asMap(value)
		return ai, nil
	}, func(resource, ai map[string]any) {
		if ai == nil {
			return
		}
		if _, ok := ai["name"]; ok {
			resource["name"] = ai["name"]
		}
		if opts.Context != "" {
			resource["context"] = opts.Context
		} else if _, ok := ai["context"]; ok {
			resource["context"] = ai["context"]
		}
		if _, ok := ai["description"]; ok {
			resource["description"] = ai["description"]
		}
		if args, ok := ai["arguments"]; ok && args != nil {
			resource["arguments"] = args
		}
	})
}

func describeWebhooks(client *api.HTTPClient, resources []map[string]any, opts GenerateOptions) ([]string, error) {
	return processResources(resources, "webhook", opts, func(resource map[string]any) (map[string]any, error) {
		if opts.DisableAI {
			return resource, nil
		}
		payload := map[string]any{
			"name":         resource["name"],
			"context":      resource["context"],
			"description":  resource["description"],
			"eventPayload": resource["eventPayloadTypeSchema"],
		}
		value, err := client.PostQuery("webhooks/description-generation", nil, payload)
		if err != nil {
			return nil, err
		}
		ai, _ := asMap(value)
		return ai, nil
	}, func(resource, ai map[string]any) {
		if ai == nil {
			return
		}
		if _, ok := ai["name"]; ok {
			resource["name"] = ai["name"]
		}
		if opts.Context != "" {
			resource["context"] = opts.Context
		} else if _, ok := ai["context"]; ok {
			resource["context"] = ai["context"]
		}
		if _, ok := ai["description"]; ok {
			resource["description"] = ai["description"]
		}
	})
}

func processResources(
	resources []map[string]any,
	kind string,
	opts GenerateOptions,
	fetch func(map[string]any) (map[string]any, error),
	apply func(resource, ai map[string]any),
) ([]string, error) {
	if len(resources) == 0 {
		return nil, nil
	}
	if opts.DisableAI {
		if opts.Context != "" {
			for _, r := range resources {
				r["context"] = opts.Context
			}
		}
		return nil, nil
	}

	results := make([]settled[map[string]any], len(resources))
	forChunks(len(resources), func(from, to int) {
		opts.progress(fmt.Sprintf("Processing from %s number %d to %s number %d out of %d", kind, from+1, kind, to, len(resources)))
		chunk := runSettled(to-from, func(i int) (map[string]any, error) {
			return fetch(resources[from+i])
		})
		copy(results[from:to], chunk)
	})

	var warnings []string
	var unnamed []int
	for i, result := range results {
		resource := resources[i]
		if result.err != nil {
			if opts.Context != "" {
				resource["context"] = opts.Context
			}
			warnings = append(warnings, formatDescribeFailure(kind, resource, i, result.err, ""))
			if mapString(resource, "name") == "" {
				unnamed = append(unnamed, i)
			}
			continue
		}
		ai := result.value
		apply(resource, ai)
		if trace := mapString(ai, "traceId"); trace != "" {
			warnings = append(warnings, formatDescribeFailure(kind, resource, i, nil, trace))
		}
		if mapString(resource, "name") == "" {
			unnamed = append(unnamed, i)
		}
	}
	for _, i := range unnamed {
		warnings = append(warnings, fmt.Sprintf("action required: %s at index %d has no name", kind, i))
	}
	return warnings, nil
}

func formatDescribeFailure(kind string, resource map[string]any, index int, err error, traceID string) string {
	label := fmt.Sprintf("%s with context %q and name %q at index %d", firstUpper(kind), mapString(resource, "context"), mapString(resource, "name"), index)
	if err != nil {
		return fmt.Sprintf("%s — %s", label, describeError(err))
	}
	return fmt.Sprintf("%s — trace id: %s", label, traceID)
}

func describeError(err error) string {
	var ae *api.Error
	if errors.As(err, &ae) && ae != nil && ae.Status() != 0 {
		msg := fmt.Sprintf("request failure with status code %d", ae.Status())
		if m := api.UserMessage(err); m != "" && m != ae.Error() {
			msg += ` - "` + m + `"`
		}
		return msg
	}
	return "request failure: " + err.Error()
}

func firstUpper(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}
