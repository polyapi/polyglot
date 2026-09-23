package cli

import "github.com/spf13/cobra"

func addCRUD(parent *cobra.Command, resource string) {
	prefix := "polyapi " + resource

	list := &cobra.Command{
		Use:   "list",
		Short: "List " + resource,
		Args:  cobra.NoArgs,
		RunE:  stub(resource + " list"),
		Example: examples(
			ex{"List all:", prefix + " list"},
			ex{"Restrict to a context:", prefix + " list --context billing"},
		),
	}
	list.Flags().String("context", "", "Restrict results to this context prefix")

	get := &cobra.Command{
		Use:   "get <id>",
		Short: "Get a " + resource + " by ID",
		Args:  cobra.ExactArgs(1),
		RunE:  stub(resource + " get"),
		Example: examples(
			ex{"Get by ID:", prefix + " get abc123"},
		),
	}

	create := &cobra.Command{
		Use:   "create",
		Short: "Create a " + resource,
		Args:  cobra.NoArgs,
		RunE:  stub(resource + " create"),
		Example: examples(
			ex{"Create with a name and context:", prefix + " create --name example --context billing"},
		),
	}
	create.Flags().String("name", "", "Resource name")
	create.Flags().String("context", "", "Resource context")
	_ = create.MarkFlagRequired("name")
	_ = create.MarkFlagRequired("context")

	update := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a " + resource,
		Args:  cobra.ExactArgs(1),
		RunE:  stub(resource + " update"),
		Example: examples(
			ex{"Update by ID:", prefix + " update abc123"},
		),
	}

	del := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a " + resource,
		Args:  cobra.ExactArgs(1),
		RunE:  stub(resource + " delete"),
		Example: examples(
			ex{"Delete by ID:", prefix + " delete abc123"},
		),
	}

	parent.AddCommand(list, get, create, update, del)
}
