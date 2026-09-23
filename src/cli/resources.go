package cli

import (
	"fmt"
	"strings"

	"github.com/polyapi/polyglot/src/glide"
	"github.com/spf13/cobra"
)

func addResourceCommands(root *cobra.Command) {
	addFunctionCommands(root)
	addVariCommands(root)
	addTableCommands(root)
	addWebhookCommands(root)
	addTriggerCommands(root)
	addJobCommands(root)
	addSchemaCommands(root)
	addSnippetCommands(root)
	addSubscriptionCommands(root)
	addAppCommands(root)
}

type functionTypeValue struct{ value string }

func (f *functionTypeValue) String() string { return f.value }

func (f *functionTypeValue) Set(s string) error {
	switch strings.ToLower(s) {
	case "server", "client", "api", "ai":
		f.value = strings.ToLower(s)
		return nil
	default:
		return fmt.Errorf(`invalid argument %q for "--type" (server|client|api|ai)`, s)
	}
}

func (f *functionTypeValue) Type() string { return "function-type" }

type triggerTypeValue struct{ value string }

func (f *triggerTypeValue) String() string { return f.value }

func (f *triggerTypeValue) Set(s string) error {
	kind, ok := glide.NormalizeTriggerSource(s)
	if !ok {
		return fmt.Errorf(`invalid argument %q for "--type" (webhook|error-handler)`, s)
	}
	f.value = kind
	return nil
}

func (f *triggerTypeValue) Type() string { return "trigger-type" }

type subscriptionTypeValue struct{ value string }

func (f *subscriptionTypeValue) String() string { return f.value }

func (f *subscriptionTypeValue) Set(s string) error {
	kind, ok := glide.NormalizeSubscriptionType(s)
	if !ok {
		return fmt.Errorf(`invalid argument %q for "--type" (CUSTOM|OHIP)`, s)
	}
	f.value = kind
	return nil
}

func (f *subscriptionTypeValue) Type() string { return "subscription-type" }
