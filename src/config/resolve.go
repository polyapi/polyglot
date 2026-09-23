package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"github.com/spf13/viper"
)

// Overlay is a single overlay value plus its origin.
type Overlay struct {
	Value  string
	Source Source
}

// Overlays are values from CLI flags or process environment.
type Overlays struct {
	BaseURL *Overlay
	APIKey  *Overlay
}

// Resolved is fully resolved config. Secrets stay in memory (never printed).
type Resolved struct {
	ProjectRoot    string
	PolyPath       string
	Instance       string
	BaseURL        string
	APIKey         string
	APIVersion     string
	Language       string
	AdapterCommand string
	File           File
	URLSource      Source
	KeySource      Source
}

// RequireCredentials returns base URL and API key or a missing-credentials error.
func (r Resolved) RequireCredentials() (url, key string, err error) {
	if r.BaseURL == "" || r.APIKey == "" {
		return "", "", missingCredentials()
	}
	return r.BaseURL, r.APIKey, nil
}

// RedactedKey is last-four stars (or "(unset)").
func (r Resolved) RedactedKey() string {
	return RedactSecret(r.APIKey)
}

// RedactedTOML dumps the resolved view with the API key replaced by last-four stars.
func (r Resolved) RedactedTOML() string {
	urlSrc := r.URLSource.String()
	if urlSrc == "unset" && r.URLSource == "" {
		urlSrc = "(unset)"
	}
	keySrc := r.KeySource.String()
	if keySrc == "unset" && r.KeySource == "" {
		keySrc = "(unset)"
	}
	return fmt.Sprintf(
		"instance = %s\nbase_url = %s\napi_version = %q\nlanguage = %s\nadapter = %s\n\napi_key = %q\nurl_source = %q\nkey_source = %q\n",
		tomlOpt(r.Instance),
		tomlOpt(r.BaseURL),
		r.APIVersion,
		tomlOpt(r.Language),
		tomlOpt(r.AdapterCommand),
		r.RedactedKey(),
		urlSrc,
		keySrc,
	)
}

func tomlOpt(value string) string {
	if value == "" {
		return `""`
	}
	return fmt.Sprintf("%q", value)
}

// Load merges layers with Viper: user file → project file → env → flag overlays.
// Legacy SDK files fill gaps Viper did not set. Encrypted keys are decrypted after merge.
func Load(projectRoot, polyPath string, overlays Overlays, secrets SecretContext) (Resolved, error) {
	userPath := userConfigFileOverride(secrets)
	projectPath := ProjectConfigFile(projectRoot, polyPath)

	user, err := readFile(userPath)
	if err != nil {
		return Resolved{}, err
	}
	project, err := readFile(projectPath)
	if err != nil {
		return Resolved{}, err
	}

	v := viper.New()
	v.SetConfigType("toml")
	if err := mergeFile(v, userPath); err != nil {
		return Resolved{}, err
	}
	if err := mergeFile(v, projectPath); err != nil {
		return Resolved{}, err
	}
	if err := v.BindEnv("api_key", "POLY_API_KEY"); err != nil {
		return Resolved{}, message(err.Error())
	}
	if err := v.BindEnv("base_url", "POLY_API_BASE_URL"); err != nil {
		return Resolved{}, message(err.Error())
	}
	if overlays.BaseURL != nil {
		v.Set("base_url", overlays.BaseURL.Value)
	}
	if overlays.APIKey != nil {
		v.Set("api_key", overlays.APIKey.Value)
	}

	file := mergeFileDocs(user, project)

	baseURL := v.GetString("base_url")
	apiKey := v.GetString("api_key")
	urlSource := sourceOf(overlays.BaseURL, "POLY_API_BASE_URL", user, project, true)
	keySource := sourceOf(overlays.APIKey, "POLY_API_KEY", nil, nil, false)
	if apiKey == "" {
		var err error
		apiKey, keySource, err = decryptFromFiles(user, project, secrets)
		if err != nil {
			return Resolved{}, err
		}
	}

	if apiKey == "" || baseURL == "" {
		if legacy := ReadLegacy(projectRoot); legacy != nil {
			if baseURL == "" && legacy.BaseURL != "" {
				baseURL = legacy.BaseURL
				urlSource = legacy.Source
			}
			if apiKey == "" && legacy.APIKey != "" {
				apiKey = legacy.APIKey
				keySource = legacy.Source
			}
		}
	}

	if baseURL != "" {
		resolved, err := ResolveBaseURL(baseURL)
		if err != nil {
			return Resolved{}, err
		}
		baseURL = resolved
	}

	instance := file.Instance
	if instance == "" {
		instance = InstanceName(baseURL)
	}

	apiVersion := file.APIVersion
	if apiVersion == "" {
		apiVersion = "1"
	}

	adapterCommand := ""
	if file.Adapter != nil {
		adapterCommand = file.Adapter.Command
	}

	return Resolved{
		ProjectRoot:    projectRoot,
		PolyPath:       polyPath,
		Instance:       instance,
		BaseURL:        baseURL,
		APIKey:         apiKey,
		APIVersion:     apiVersion,
		Language:       file.Language,
		AdapterCommand: adapterCommand,
		File:           file,
		URLSource:      urlSource,
		KeySource:      keySource,
	}, nil
}

func mergeFile(v *viper.Viper, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return ioError(path, err)
	}
	if err := v.MergeConfig(bytes.NewReader(raw)); err != nil {
		return tomlError(path, err)
	}
	return nil
}

func mergeFileDocs(user, project *File) File {
	var file File
	applyDoc(&file, user)
	applyDoc(&file, project)
	return file
}

func applyDoc(dest *File, incoming *File) {
	if incoming == nil {
		return
	}
	if incoming.Instance != "" {
		dest.Instance = incoming.Instance
	}
	if incoming.Language != "" {
		dest.Language = incoming.Language
	}
	if incoming.Adapter != nil && incoming.Adapter.Command != "" {
		dest.Adapter = incoming.Adapter
	}
	if incoming.APIVersion != "" {
		dest.APIVersion = incoming.APIVersion
	}
	if len(incoming.Environments) > 0 {
		dest.Environments = incoming.Environments
	}
	if incoming.Deploy != nil {
		dest.Deploy = incoming.Deploy
	}
	if incoming.BaseURL != "" {
		dest.BaseURL = incoming.BaseURL
	}
	if incoming.Auth != nil {
		dest.Auth = incoming.Auth
	}
}

func sourceOf(overlay *Overlay, envKey string, user, project *File, url bool) Source {
	if overlay != nil {
		return overlay.Source
	}
	if os.Getenv(envKey) != "" {
		return SourceEnv
	}
	if url {
		if project != nil && project.BaseURL != "" {
			return SourceProject
		}
		if user != nil && user.BaseURL != "" {
			return SourceUser
		}
	}
	return ""
}

func decryptFromFiles(user, project *File, secrets SecretContext) (string, Source, error) {
	for _, pair := range []struct {
		file   *File
		source Source
	}{
		{project, SourceProject},
		{user, SourceUser},
	} {
		if pair.file == nil || pair.file.Auth == nil || pair.file.Auth.APIKeyEncrypted == "" {
			continue
		}
		secret, err := DeviceSecret(secrets)
		if err != nil {
			return "", "", err
		}
		plain, err := DecryptAPIKey(pair.file.Auth.APIKeyEncrypted, secret)
		if err != nil {
			return "", "", err
		}
		return plain, pair.source, nil
	}
	return "", "", nil
}

func userConfigFileOverride(secrets SecretContext) string {
	if secrets.UseKeyring {
		return UserConfigFile()
	}
	return filepath.Join(secrets.UserConfigDir, "config.toml")
}

func readFile(path string) (*File, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("toml")
	if err := v.ReadInConfig(); err != nil {
		if isConfigNotFound(err) {
			return nil, nil
		}
		return nil, tomlError(path, err)
	}
	var parsed File
	if err := v.Unmarshal(&parsed); err != nil {
		return nil, tomlError(path, err)
	}
	return &parsed, nil
}

func isConfigNotFound(err error) bool {
	var notFound viper.ConfigFileNotFoundError
	if errors.As(err, &notFound) {
		return true
	}
	return os.IsNotExist(err)
}

// SaveProjectCredentials writes instance / URL / encrypted API key into the project config file.
func SaveProjectCredentials(projectRoot, polyPath, baseURL, apiKey, apiVersion string, secrets SecretContext) (string, error) {
	path := ProjectConfigFile(projectRoot, polyPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", ioError(filepath.Dir(path), err)
	}
	file, err := readFile(path)
	if err != nil {
		return "", err
	}
	if file == nil {
		file = &File{}
	}
	url, err := ResolveBaseURL(baseURL)
	if err != nil {
		return "", err
	}
	file.BaseURL = url
	file.Instance = InstanceName(url)
	file.APIVersion = apiVersion
	secret, err := DeviceSecret(secrets)
	if err != nil {
		return "", err
	}
	encrypted, err := EncryptAPIKey(apiKey, secret)
	if err != nil {
		return "", err
	}
	file.Auth = &AuthFile{APIKeyEncrypted: encrypted}
	if err := writeFile(path, file); err != nil {
		return "", err
	}
	return path, nil
}

// ClearStoredKeys removes stored API keys from project (and user) config. Leaves URLs in place.
func ClearStoredKeys(projectRoot, polyPath string, secrets SecretContext) error {
	for _, path := range []string{
		ProjectConfigFile(projectRoot, polyPath),
		userConfigFileOverride(secrets),
	} {
		file, err := readFile(path)
		if err != nil {
			return err
		}
		if file == nil || file.Auth == nil {
			continue
		}
		file.Auth = nil
		if err := writeFile(path, file); err != nil {
			return err
		}
	}
	return nil
}

// Setting returns one resolved value. api_key is always redacted.
func (r Resolved) Setting(key string) (string, error) {
	switch normalizeSettingKey(key) {
	case "instance":
		return r.Instance, nil
	case "base_url":
		return r.BaseURL, nil
	case "api_version":
		return r.APIVersion, nil
	case "language":
		return r.Language, nil
	case "adapter", "adapter.command":
		return r.AdapterCommand, nil
	case "api_key":
		return r.RedactedKey(), nil
	case "url_source":
		return r.URLSource.String(), nil
	case "key_source":
		return r.KeySource.String(), nil
	default:
		return "", unknownSetting(key)
	}
}

// SetProjectSetting writes a non-secret key into the project config file.
func SetProjectSetting(projectRoot, polyPath, key, value string) error {
	switch normalizeSettingKey(key) {
	case "api_key", "url_source", "key_source":
		return secretSetting(key)
	case "instance", "base_url", "api_version", "language", "adapter", "adapter.command":
	default:
		return unknownSetting(key)
	}
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
	switch normalizeSettingKey(key) {
	case "language":
		file.Language = value
	case "adapter", "adapter.command":
		if value == "" {
			file.Adapter = nil
		} else {
			file.Adapter = &AdapterFile{Command: value}
		}
	case "api_version":
		file.APIVersion = value
	case "base_url", "instance":
		url, err := ResolveBaseURL(value)
		if err != nil {
			return err
		}
		file.BaseURL = url
		file.Instance = InstanceName(url)
	}
	return writeFile(path, file)
}

func normalizeSettingKey(key string) string {
	s := strings.TrimSpace(strings.ToLower(key))
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func writeFile(path string, file *File) error {
	raw, err := toml.Marshal(file)
	if err != nil {
		return message("serialize config: " + err.Error())
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		return ioError(path, err)
	}
	return nil
}
