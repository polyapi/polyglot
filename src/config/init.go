package config

import (
	"os"
	"path/filepath"
	"strings"
)

// InitSpec is the project config `polyapi init` writes or merges.
type InitSpec struct {
	Language      string
	BaseURL       string
	APIKey        string
	APIVersion    string
	EnvMap        []string // "main=prod"
	ReplaceDeploy bool     // overwrite existing deploy.targets
}

// ApplyInit writes language, deploy.targets, environments, and optional credentials.
func ApplyInit(projectRoot, polyPath string, spec InitSpec, secrets SecretContext) error {
	path := ProjectConfigFile(projectRoot, polyPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ioError(filepath.Dir(path), err)
	}
	file, err := readFile(path)
	if err != nil {
		return err
	}
	if file == nil {
		file = &File{}
	}
	if spec.Language != "" {
		file.Language = spec.Language
	}
	if spec.APIVersion != "" {
		file.APIVersion = spec.APIVersion
	} else if file.APIVersion == "" {
		file.APIVersion = "1"
	}
	if spec.BaseURL != "" {
		url, err := ResolveBaseURL(spec.BaseURL)
		if err != nil {
			return err
		}
		file.BaseURL = url
		file.Instance = InstanceName(url)
	}
	if spec.APIKey != "" && spec.BaseURL != "" {
		if _, err := SaveProjectCredentials(projectRoot, polyPath, spec.BaseURL, spec.APIKey, file.APIVersion, secrets); err != nil {
			return err
		}
		// Re-read after credentials so we merge onto the saved file.
		file, err = readFile(path)
		if err != nil {
			return err
		}
		if file == nil {
			file = &File{}
		}
		if spec.Language != "" {
			file.Language = spec.Language
		}
	}
	if spec.ReplaceDeploy || file.Deploy == nil || len(file.Deploy.Targets) == 0 {
		targets, envs := parseEnvMap(spec.EnvMap, file.BaseURL)
		if file.Deploy == nil {
			file.Deploy = &DeployFile{}
		}
		file.Deploy.UnallowedBranch = "error"
		file.Deploy.Targets = targets
		if file.Environments == nil {
			file.Environments = map[string]EnvironmentFile{}
		}
		for name, env := range envs {
			if existing, ok := file.Environments[name]; ok && existing.BaseURL != "" && env.BaseURL == "" {
				continue
			}
			file.Environments[name] = env
		}
	}
	return writeFile(path, file)
}

func parseEnvMap(items []string, defaultURL string) ([]DeployTargetFile, map[string]EnvironmentFile) {
	if len(items) == 0 {
		items = []string{"main=prod"}
	}
	var targets []DeployTargetFile
	envs := map[string]EnvironmentFile{}
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		branch, env, ok := strings.Cut(item, "=")
		if !ok {
			branch, env = item, "prod"
		}
		branch = strings.TrimSpace(branch)
		env = strings.TrimSpace(env)
		if branch == "" || env == "" {
			continue
		}
		prod := strings.EqualFold(env, "prod") || strings.EqualFold(env, "production")
		t := DeployTargetFile{
			Branches:    []string{branch},
			Environment: env,
		}
		if prod {
			t.Production = &prod
		}
		targets = append(targets, t)
		if _, exists := envs[env]; !exists {
			envs[env] = EnvironmentFile{BaseURL: defaultURL}
		}
	}
	if len(targets) == 0 {
		prod := true
		targets = []DeployTargetFile{{
			Branches:    []string{"main"},
			Environment: "prod",
			Production:  &prod,
		}}
		envs["prod"] = EnvironmentFile{BaseURL: defaultURL}
	}
	return targets, envs
}
