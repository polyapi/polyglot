package glide

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

const scaffoldExt = ".jsonc"

// ArtifactDir is the folder name under artifacts/ for typ (vari, tabi, server, …).
func ArtifactDir(typ string) string {
	return artefactsDir(typ)
}

// ArtifactRel is the default local JSONC path for a resource of typ.
// Context dots become directories: billing.orders → src/billing/orders/artifacts/….
// Jobs and triggers (no context) land under src/artifacts/<type>/.
func ArtifactRel(typ, context, name string) string {
	dir := ArtifactDir(typ)
	ctxPath := strings.ReplaceAll(strings.TrimSpace(context), ".", string(filepath.Separator))
	if ctxPath == "" {
		return filepath.ToSlash(filepath.Join("src", "artifacts", dir, name+scaffoldExt))
	}
	return filepath.ToSlash(filepath.Join("src", ctxPath, "artifacts", dir, name+scaffoldExt))
}

// ScaffoldJSONC returns a commented JSONC artifact with placeholders for every
// authorable field of typ. name and context are filled in; other fields are
// empty or type-appropriate defaults so the file is valid to load, then edit.
func ScaffoldJSONC(typ, name, context string) (string, error) {
	if _, ok := NormalizeType(typ); !ok && !IsFunction(typ) {
		return "", fmt.Errorf("unknown resource type %q", typ)
	}
	n := jq(name)
	c := jq(context)
	switch typ {
	case TypeVariable:
		return scaffoldVariable(n, c), nil
	case TypeTable:
		return scaffoldTable(n, c), nil
	case TypeSchema:
		return scaffoldSchema(n, c), nil
	case TypeWebhook:
		return scaffoldWebhook(n, c), nil
	case TypeTrigger:
		return "", fmt.Errorf("triggers use ScaffoldTriggerJSONC, not ScaffoldJSONC")
	case TypeSubscription:
		return "", fmt.Errorf("subscriptions use ScaffoldSubscriptionJSONC, not ScaffoldJSONC")
	case TypeJob:
		return scaffoldJob(n), nil
	case TypeSnippet:
		return scaffoldSnippet(n, c), nil
	case TypeApplication:
		return scaffoldApplication(n), nil
	case TypeAPIFunction, TypeAIFunction:
		return scaffoldFunction(typ, n, c), nil
	case TypeServerFunction, TypeClientFunction:
		return "", fmt.Errorf("server and client functions use ScaffoldFunctionCode, not JSONC")
	default:
		return "", fmt.Errorf("no local scaffold for type %q", typ)
	}
}

func jq(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(b)
}

func scaffoldHeader(noun, extra string) string {
	line := "// Local " + noun + " scaffold. Fill in the placeholders, then `polyapi deploy plan`."
	if extra != "" {
		return line + "\n// " + extra + "\n"
	}
	return line + "\n"
}

func scaffoldVariable(name, context string) string {
	return scaffoldHeader("variable", "SECRET values in source are never pushed (value becomes {}).") + `{
  "polyType": "variable",
  "name": ` + name + `,
  "context": ` + context + `,
  "description": "",
  "visibility": "ENVIRONMENT",
  "secrecy": "NONE",
  "value": {},
  "expiresAt": null
}
`
}

func scaffoldTable(name, context string) string {
	return scaffoldHeader("table", "Do not add id / created_at / updated_at — Tabi manages those. Visibility is ENVIRONMENT or TENANT.") + `{
  "polyType": "table",
  "name": ` + name + `,
  "context": ` + context + `,
  "description": "",
  "visibility": "ENVIRONMENT",
  "columns": [
    {
      "name": "example",
      "type": "text",
      "required": false,
      "unique": false
    }
  ]
}
`
}

func scaffoldSchema(name, context string) string {
	return scaffoldHeader("schema", "Other schemas are referenced with x-poly-ref {path} (optional publicNamespace).") + `{
  "polyType": "schema",
  "name": ` + name + `,
  "context": ` + context + `,
  "visibility": "ENVIRONMENT",
  "definition": {
    "$schema": "http://json-schema.org/draft-06/schema#",
    "type": "object",
    "properties": {}
  }
}
`
}

func scaffoldWebhook(name, context string) string {
	return scaffoldHeader("webhook", "securityFunctions entries may be context.name; the host rewrites them to server-function ids on deploy.") + `{
  "polyType": "webhook",
  "name": ` + name + `,
  "context": ` + context + `,
  "description": "",
  "visibility": "ENVIRONMENT",
  "enabled": true,
  "state": "",
  "method": "POST",
  "slug": "",
  "subpath": "",
  "requirePolyApiKey": false,
  "eventPayload": {},
  "eventPayloadType": "",
  "eventPayloadTypeSchema": {},
  "responsePayload": {},
  "responseHeaders": {},
  "responseStatus": 200,
  "securityFunctions": [],
  "xmlParserOptions": {}
}
`
}

// NormalizeTriggerSource maps CLI aliases to TriggerSourceWebhook or
// TriggerSourceErrorHandler.
func NormalizeTriggerSource(s string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case TriggerSourceWebhook:
		return TriggerSourceWebhook, true
	case TriggerSourceErrorHandler, "error_handler", "errorhandler":
		return TriggerSourceErrorHandler, true
	default:
		return "", false
	}
}

// DetectTriggerSource returns webhook or error-handler from a trigger payload.
// Both kinds at once, or neither, is an error.
func DetectTriggerSource(m map[string]any) (string, error) {
	src, _ := asObject(m["source"])
	wh := stringField(src, "webhookHandleId")
	if wh == "" {
		wh = stringField(m, "webhookHandleId")
	}
	hasWebhook := wh != "" || (stringField(src, "webhookContext") != "" && stringField(src, "webhookName") != "")
	hasEH := stringField(firstMap(m, src, "errorHandler"), "path") != ""
	switch {
	case hasWebhook && hasEH:
		return "", fmt.Errorf("snippet source has both webhook and error-handler; use a snippet with a single source")
	case hasEH:
		return TriggerSourceErrorHandler, nil
	case hasWebhook:
		return TriggerSourceWebhook, nil
	default:
		return "", fmt.Errorf("snippet is not a webhook or error-handler trigger")
	}
}

// CheckTriggerJSONCSource reports whether JSONC body matches want
// (webhook|error-handler, including aliases).
func CheckTriggerJSONCSource(body, want string) error {
	kind, ok := NormalizeTriggerSource(want)
	if !ok {
		return fmt.Errorf("invalid trigger type %q (webhook|error-handler)", want)
	}
	stripped := strings.TrimSpace(StripJSONC(body))
	var obj map[string]any
	if err := json.Unmarshal([]byte(stripped), &obj); err != nil {
		return fmt.Errorf("snippet code is not valid JSON: %w", err)
	}
	got, err := DetectTriggerSource(obj)
	if err != nil {
		return err
	}
	if got != kind {
		return fmt.Errorf("snippet source is %s, not %s required by --type", got, kind)
	}
	return nil
}

// ScaffoldTriggerJSONC is the JSONC artifact for a webhook- or
// error-handler-sourced trigger.
func ScaffoldTriggerJSONC(name, source string) (string, error) {
	kind, ok := NormalizeTriggerSource(source)
	if !ok {
		return "", fmt.Errorf("invalid trigger type %q (webhook|error-handler)", source)
	}
	n := jq(name)
	if kind == TriggerSourceErrorHandler {
		return scaffoldHeader("trigger", "Triggers have no context. Replace the error-handler path and destination function with context.name (or ids). Error-handler triggers do not set waitForResponse.") + `{
  "polyType": "trigger",
  "name": ` + n + `,
  "enabled": true,
  "source": {
    "errorHandler": {
      "path": "example.handler"
    }
  },
  "destination": {
    "serverFunctionId": "example.handler"
  }
}
`, nil
	}
	return scaffoldHeader("trigger", "Triggers have no context. Replace the source webhook and destination function with context.name (or ids).") + `{
  "polyType": "trigger",
  "name": ` + n + `,
  "enabled": true,
  "waitForResponse": true,
  "source": {
    "webhookHandleId": "example.hook"
  },
  "destination": {
    "serverFunctionId": "example.handler"
  }
}
`, nil
}

// NormalizeSubscriptionType maps CLI aliases to CUSTOM or OHIP.
func NormalizeSubscriptionType(s string) (string, bool) {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case SubscriptionTypeCustom:
		return SubscriptionTypeCustom, true
	case SubscriptionTypeOHIP:
		return SubscriptionTypeOHIP, true
	default:
		return "", false
	}
}

// ValidSubscriptionName is the platform NameIdentifier: letter or underscore
// start, then letters, digits, or underscore. Hyphens are rejected.
func ValidSubscriptionName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	for i, r := range name {
		ok := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return fmt.Errorf("subscription name %q must start with a letter or underscore and contain only letters, digits, and underscores", name)
		}
	}
	return nil
}

// CheckSubscriptionJSONCType ensures a snippet's type matches --type.
func CheckSubscriptionJSONCType(body, want string) error {
	kind, ok := NormalizeSubscriptionType(want)
	if !ok {
		return fmt.Errorf("invalid subscription type %q (CUSTOM|OHIP)", want)
	}
	stripped := StripJSONC(body)
	var obj map[string]any
	if err := json.Unmarshal([]byte(stripped), &obj); err != nil {
		return fmt.Errorf("snippet is not JSONC: %w", err)
	}
	got := strings.TrimSpace(stringField(obj, "type"))
	if got == "" {
		return nil
	}
	normalized, ok := NormalizeSubscriptionType(got)
	if !ok || normalized != kind {
		return fmt.Errorf("snippet type is %s, not %s required by --type", got, kind)
	}
	return nil
}

// ScaffoldSubscriptionJSONC is the JSONC artifact for a CUSTOM or OHIP subscription.
func ScaffoldSubscriptionJSONC(name, context, source string) (string, error) {
	kind, ok := NormalizeSubscriptionType(source)
	if !ok {
		return "", fmt.Errorf("invalid subscription type %q (CUSTOM|OHIP)", source)
	}
	if err := ValidSubscriptionName(name); err != nil {
		return "", err
	}
	n := jq(name)
	c := jq(context)
	if kind == SubscriptionTypeOHIP {
		return scaffoldHeader("GraphQL subscription", "OHIP streams are at-least-once. Keep paramsObject.ohip secrets in {{ENV_TOKEN}} form. ohipOffset is runtime state and is not deployed.") + `{
  "polyType": "subscription",
  "name": ` + n + `,
  "context": ` + c + `,
  "description": "",
  "visibility": "ENVIRONMENT",
  "type": "OHIP",
  "transportProtocol": "WS",
  "websocketUrl": "wss://example-ohip.example/graphql",
  "query": "subscription StreamUpdates { newEvent(input: { chainCode: \"CHA\" }) { moduleName eventName metadata { offset uniqueEventId } } }",
  "functionId": "example.handler",
  "functionParams": null,
  "paramsObject": {
    "ohip": {
      "hostName": "{{OHIP_HOST}}",
      "appKey": "{{OHIP_APP_KEY}}",
      "enterpriseId": "{{OHIP_ENTERPRISE_ID}}",
      "clientId": "{{OHIP_CLIENT_ID}}",
      "clientSecret": "{{OHIP_CLIENT_SECRET}}"
    }
  },
  "ohipMaintainOffset": true,
  "enabled": true,
  "eventInactivityThresholdMs": null,
  "queueId": null
}
`, nil
	}
	return scaffoldHeader("GraphQL subscription", "Replace functionId with a server-function context.name (or id). Use paramsVariableId, paramsSfxId, or paramsObject — at most one.") + `{
  "polyType": "subscription",
  "name": ` + n + `,
  "context": ` + c + `,
  "description": "",
  "visibility": "ENVIRONMENT",
  "type": "CUSTOM",
  "transportProtocol": "WS",
  "websocketUrl": "wss://example-provider.com/graphql",
  "query": "subscription Example { exampleUpdated { id } }",
  "functionId": "example.handler",
  "functionParams": null,
  "paramsVariableId": null,
  "paramsSfxId": null,
  "paramsObject": null,
  "enabled": true,
  "eventInactivityThresholdMs": null,
  "queueId": null
}
`, nil
}

func scaffoldJob(name string) string {
	return scaffoldHeader("job", "Jobs have no context. Replace functions with context.name (functionContext + functionName) or a server-function id. Omit schedule for a manual-only job.") + `{
  "polyType": "job",
  "name": ` + name + `,
  "enabled": true,
  "executionType": "sequential",
  "schedule": {
    "type": "periodical",
    "value": "0 0 * * *"
  },
  "functions": [
    {
      "functionContext": "example",
      "functionName": "handler"
    }
  ]
}
`
}

func scaffoldSnippet(name, context string) string {
	return scaffoldHeader("snippet", "language is lowercase (typescript, javascript, python, …).") + `{
  "polyType": "snippet",
  "name": ` + name + `,
  "context": ` + context + `,
  "description": "",
  "visibility": "ENVIRONMENT",
  "language": "typescript",
  "code": "// snippet body\n"
}
`
}

func scaffoldApplication(name string) string {
	return scaffoldHeader("application", "Applications have no context. config is the Canopy UI definition (name, subpath, collections). Map collection list/get to Poly functions.") + `{
  "polyType": "application",
  "name": ` + name + `,
  "description": "",
  "visibility": "ENVIRONMENT",
  "config": {
    "name": ` + name + `,
    "subpath": "",
    "icon": "",
    "logoSrc": "",
    "login": {
      "title": "",
      "logoSrc": ""
    },
    "collections": []
  }
}
`
}

func scaffoldFunction(typ, name, context string) string {
	extra := "Function type is " + typ + ". Code modules with polyConfig are preferred once a language adapter is installed; this JSONC artifact is deployable without one."
	var b strings.Builder
	b.WriteString(scaffoldHeader(typ, extra))
	b.WriteString("{\n")
	b.WriteString("  \"polyType\": " + jq(typ) + ",\n")
	b.WriteString("  \"name\": " + name + ",\n")
	b.WriteString("  \"context\": " + context + ",\n")
	b.WriteString("  \"description\": \"\",\n")
	b.WriteString("  \"visibility\": \"ENVIRONMENT\",\n")
	b.WriteString("  \"language\": \"typescript\",\n")
	b.WriteString("  \"code\": \"export async function run() {\\n  return null;\\n}\"")
	if typ == TypeServerFunction {
		b.WriteString(",\n")
		b.WriteString("  \"logsEnabled\": true,\n")
		b.WriteString("  \"image\": \"\",\n")
		b.WriteString("  \"generateContexts\": \"\"")
	}
	b.WriteString("\n}\n")
	return b.String()
}
