package config

// File is an on-disk TOML config.toml document. Secrets are only stored encrypted.
type File struct {
	Instance     string                     `mapstructure:"instance" toml:"instance,omitempty"`
	BaseURL      string                     `mapstructure:"base_url" toml:"base_url,omitempty"`
	Language     string                     `mapstructure:"language" toml:"language,omitempty"`
	APIVersion   string                     `mapstructure:"api_version" toml:"api_version,omitempty"`
	Auth         *AuthFile                  `mapstructure:"auth" toml:"auth,omitempty"`
	Adapter      *AdapterFile               `mapstructure:"adapter" toml:"adapter,omitempty"`
	Environments map[string]EnvironmentFile `mapstructure:"environments" toml:"environments,omitempty"`
	Deploy       *DeployFile                `mapstructure:"deploy" toml:"deploy,omitempty"`
}

// AuthFile holds encrypted credentials. Never contains a plaintext API key.
type AuthFile struct {
	APIKeyEncrypted string `mapstructure:"api_key_encrypted" toml:"api_key_encrypted,omitempty"`
}

// AdapterFile is an optional adapter launch override.
type AdapterFile struct {
	Command string `mapstructure:"command" toml:"command,omitempty"`
}

// EnvironmentFile is a named environment ([environments.dev]).
type EnvironmentFile struct {
	BaseURL string `mapstructure:"base_url" toml:"base_url,omitempty"`
}

// DeployFile is committed (or local) deploy policy.
type DeployFile struct {
	UnallowedBranch string             `mapstructure:"unallowed_branch" toml:"unallowed_branch,omitempty"`
	RequireGit      *bool              `mapstructure:"require_git" toml:"require_git,omitempty"`
	Receipts        *bool              `mapstructure:"receipts" toml:"receipts,omitempty"`
	Targets         []DeployTargetFile `mapstructure:"targets" toml:"targets,omitempty"`
	Scope           *DeployScopeFile   `mapstructure:"scope" toml:"scope,omitempty"`
}

// DeployTargetFile is one branch → environment mapping.
type DeployTargetFile struct {
	Branches    []string `mapstructure:"branches" toml:"branches,omitempty"`
	Environment string   `mapstructure:"environment" toml:"environment,omitempty"`
	Production  *bool    `mapstructure:"production" toml:"production,omitempty"`
}

// DeployScopeFile is path / type / context filters.
type DeployScopeFile struct {
	Paths                 []string          `mapstructure:"paths" toml:"paths,omitempty"`
	ExcludePaths          []string          `mapstructure:"exclude_paths" toml:"exclude_paths,omitempty"`
	Types                 []string          `mapstructure:"types" toml:"types,omitempty"`
	Contexts              []string          `mapstructure:"contexts" toml:"contexts,omitempty"`
	ContextPathMap        map[string]string `mapstructure:"context_path_map" toml:"context_path_map,omitempty"`
	ExcludeOrphanContexts []string          `mapstructure:"exclude_orphan_contexts" toml:"exclude_orphan_contexts,omitempty"`
}
