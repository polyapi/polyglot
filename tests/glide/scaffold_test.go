package glide_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/polyapi/polyglot/src/glide"
)

func TestArtifactRel(t *testing.T) {
	got := glide.ArtifactRel(glide.TypeVariable, "billing.orders", "apiKey")
	want := "src/billing/orders/artifacts/vari/apiKey.jsonc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = glide.ArtifactRel(glide.TypeJob, "", "nightly")
	want = "src/artifacts/jobs/nightly.jsonc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = glide.ArtifactRel(glide.TypeApplication, "", "dashboard")
	want = "src/artifacts/applications/dashboard.jsonc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = glide.ArtifactRel(glide.TypeAPIFunction, "billing", "proxy")
	want = "src/billing/artifacts/api/proxy.jsonc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = glide.ArtifactRel(glide.TypeSubscription, "shopify", "ordersStream")
	want = "src/shopify/artifacts/subscriptions/ordersStream.jsonc"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestFunctionCodeRel(t *testing.T) {
	got := glide.FunctionCodeRel(glide.TypeServerFunction, "billing.orders", "helloWorld", ".ts")
	want := "src/billing/orders/server/helloWorld.ts"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	got = glide.FunctionCodeRel(glide.TypeClientFunction, "", "greet", ".py")
	want = "src/client/greet.py"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestScaffoldJSONCLoadsAsDeployable(t *testing.T) {
	cases := []struct {
		typ, name, context string
	}{
		{glide.TypeVariable, "apiKey", "billing"},
		{glide.TypeTable, "orders", "billing"},
		{glide.TypeSchema, "Order", "billing"},
		{glide.TypeWebhook, "hook", "billing"},
		{glide.TypeTrigger, "weekly", ""},
		{glide.TypeJob, "nightly", ""},
		{glide.TypeSnippet, "header", "billing"},
		{glide.TypeSubscription, "ordersStream", "shopify"},
		{glide.TypeApplication, "dashboard", ""},
		{glide.TypeAPIFunction, "proxy", "billing"},
		{glide.TypeAIFunction, "summarize", "billing"},
	}
	root := t.TempDir()
	for _, c := range cases {
		var body string
		var err error
		if c.typ == glide.TypeTrigger {
			body, err = glide.ScaffoldTriggerJSONC(c.name, glide.TriggerSourceWebhook)
		} else if c.typ == glide.TypeSubscription {
			body, err = glide.ScaffoldSubscriptionJSONC(c.name, c.context, glide.SubscriptionTypeCustom)
		} else {
			body, err = glide.ScaffoldJSONC(c.typ, c.name, c.context)
		}
		if err != nil {
			t.Fatalf("%s: %v", c.typ, err)
		}
		stripped := glide.StripJSONC(body)
		var obj map[string]any
		if err := json.Unmarshal([]byte(stripped), &obj); err != nil {
			t.Fatalf("%s invalid JSONC: %v\n%s", c.typ, err, body)
		}
		if obj["polyType"] != c.typ {
			t.Fatalf("%s polyType=%v", c.typ, obj["polyType"])
		}
		if obj["name"] != c.name {
			t.Fatalf("%s name=%v", c.typ, obj["name"])
		}
		if c.context != "" && obj["context"] != c.context {
			t.Fatalf("%s context=%v", c.typ, obj["context"])
		}
		rel := glide.ArtifactRel(c.typ, c.context, c.name)
		writeJSON(t, root, rel, body)
	}
	found, err := glide.Find(root, glide.MergeScope(nil, nil, nil, nil, nil), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != len(cases) {
		t.Fatalf("found %d want %d: %+v", len(found), len(cases), rels(found))
	}
}

func TestScaffoldJSONCQuotesName(t *testing.T) {
	body, err := glide.ScaffoldJSONC(glide.TypeVariable, `say "hi"`, "billing")
	if err != nil {
		t.Fatal(err)
	}
	stripped := glide.StripJSONC(body)
	var obj map[string]any
	if err := json.Unmarshal([]byte(stripped), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["name"] != `say "hi"` {
		t.Fatalf("%v", obj["name"])
	}
}

func TestScaffoldUnknownType(t *testing.T) {
	if _, err := glide.ScaffoldJSONC("nope", "x", "y"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := glide.ScaffoldJSONC(glide.TypeTrigger, "weekly", ""); err == nil {
		t.Fatal("expected trigger to require ScaffoldTriggerJSONC")
	}
	if _, err := glide.ScaffoldJSONC(glide.TypeSubscription, "ordersStream", "shopify"); err == nil {
		t.Fatal("expected subscription to require ScaffoldSubscriptionJSONC")
	}
}

func TestCheckTriggerJSONCSource(t *testing.T) {
	wh, err := glide.ScaffoldTriggerJSONC("weekly", "webhook")
	if err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckTriggerJSONCSource(wh, "webhook"); err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckTriggerJSONCSource(wh, "error-handler"); err == nil {
		t.Fatal("expected webhook vs error-handler mismatch")
	}

	eh, err := glide.ScaffoldTriggerJSONC("on-error", "error-handler")
	if err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckTriggerJSONCSource(eh, "error_handler"); err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckTriggerJSONCSource(eh, "webhook"); err == nil {
		t.Fatal("expected error-handler vs webhook mismatch")
	}

	ctxName := `{"source":{"webhookContext":"billing","webhookName":"hook"}}`
	if err := glide.CheckTriggerJSONCSource(ctxName, "webhook"); err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckTriggerJSONCSource(`{"name":"x"}`, "webhook"); err == nil {
		t.Fatal("expected missing source")
	}
	both := `{"source":{"webhookHandleId":"a","errorHandler":{"path":"b"}}}`
	if err := glide.CheckTriggerJSONCSource(both, "webhook"); err == nil {
		t.Fatal("expected both-kinds error")
	}
}

func TestCheckSubscriptionJSONCType(t *testing.T) {
	custom, err := glide.ScaffoldSubscriptionJSONC("ordersStream", "shopify", "CUSTOM")
	if err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckSubscriptionJSONCType(custom, "custom"); err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckSubscriptionJSONCType(custom, "OHIP"); err == nil {
		t.Fatal("expected CUSTOM vs OHIP mismatch")
	}
	ohip, err := glide.ScaffoldSubscriptionJSONC("operaEvents", "opera", "OHIP")
	if err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckSubscriptionJSONCType(ohip, "OHIP"); err != nil {
		t.Fatal(err)
	}
	if err := glide.CheckSubscriptionJSONCType(`{"name":"x"}`, "CUSTOM"); err != nil {
		t.Fatal(err)
	}
}

func TestScaffoldTriggerJSONCKinds(t *testing.T) {
	body, err := glide.ScaffoldTriggerJSONC("weekly", "webhook")
	if err != nil {
		t.Fatal(err)
	}
	var wh map[string]any
	if err := json.Unmarshal([]byte(glide.StripJSONC(body)), &wh); err != nil {
		t.Fatal(err)
	}
	src, _ := wh["source"].(map[string]any)
	if src["webhookHandleId"] != "example.hook" {
		t.Fatalf("%v", src)
	}

	body, err = glide.ScaffoldTriggerJSONC("on-error", "error_handler")
	if err != nil {
		t.Fatal(err)
	}
	var eh map[string]any
	if err := json.Unmarshal([]byte(glide.StripJSONC(body)), &eh); err != nil {
		t.Fatal(err)
	}
	src, _ = eh["source"].(map[string]any)
	handler, _ := src["errorHandler"].(map[string]any)
	if handler["path"] != "example.handler" {
		t.Fatalf("%v", src)
	}
	if _, ok := eh["waitForResponse"]; ok {
		t.Fatalf("error-handler should omit waitForResponse: %v", eh)
	}

	if _, err := glide.ScaffoldTriggerJSONC("x", "cron"); err == nil {
		t.Fatal("expected error")
	}
}

func rels(found []glide.Candidate) []string {
	out := make([]string, len(found))
	for i, c := range found {
		out[i] = c.Rel
	}
	return out
}

func TestScaffoldFileOnDisk(t *testing.T) {
	root := t.TempDir()
	body, err := glide.ScaffoldJSONC(glide.TypeVariable, "apiKey", "billing")
	if err != nil {
		t.Fatal(err)
	}
	rel := glide.ArtifactRel(glide.TypeVariable, "billing", "apiKey")
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, `"secrecy": "NONE"`) {
		t.Fatalf("missing secrecy field:\n%s", body)
	}
}
