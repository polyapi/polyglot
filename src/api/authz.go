package api

// AuthData is the GET /auth body subset used by the CLI.
type AuthData struct {
	Tenant      *NamedID        `json:"tenant"`
	Environment *NamedID        `json:"environment"`
	Permissions map[string]bool `json:"permissions"`
}

// NamedID is an `{ "id": "…" }` object on /auth.
type NamedID struct {
	ID string `json:"id"`
}

// PermissionReq is one permission required to run a command.
type PermissionReq struct {
	Permission string
	Action     string
}

// ActionFor is the TS PERMISSION_ACTIONS mapping.
func ActionFor(permission string) string {
	switch permission {
	case "libraryGenerate":
		return "generate Poly library"
	case "customDev":
		return "add Client or Server functions"
	case "manageApiFunctions":
		return "add API functions"
	case "manageSchemas":
		return "add schemas"
	case "manageWebhooks":
		return "add webhooks"
	case "manageTriggers":
		return "manage triggers"
	case "manageJobs":
		return "manage jobs"
	case "manageGraphQLSubscriptions":
		return "manage GraphQL subscriptions"
	case "manageSnippets":
		return "manage snippets"
	case "manageApplications":
		return "manage applications"
	case "useApplications":
		return "use applications"
	case "manageTables":
		return "manage tables"
	case "queryTables":
		return "query tables"
	case "manageSecretVariables":
		return "manage secret variables"
	case "manageNonSecretVariables":
		return "manage non-secret variables"
	default:
		return "perform this action"
	}
}

// Requirement builds a PermissionReq from a permission name.
func Requirement(permission string) PermissionReq {
	return PermissionReq{Permission: permission, Action: ActionFor(permission)}
}

// RequirePermissions fails with a TS-style 403 message when any required permission is missing.
func RequirePermissions(auth AuthData, reqs []PermissionReq) error {
	if len(reqs) == 0 {
		return nil
	}
	var missing []PermissionReq
	for _, r := range reqs {
		if auth.Permissions[r.Permission] {
			continue
		}
		missing = append(missing, r)
	}
	if len(missing) == 0 {
		return nil
	}
	actions := make([]string, 0, len(missing))
	perms := make([]string, 0, len(missing))
	for _, m := range missing {
		actions = append(actions, m.Action)
		perms = append(perms, m.Permission)
	}
	return forbidden(joinActions(actions), perms, "")
}

func joinActions(actions []string) string {
	out := ""
	for i, a := range actions {
		if i > 0 {
			out += " | "
		}
		out += a
	}
	return out
}
