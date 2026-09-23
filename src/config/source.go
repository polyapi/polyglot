package config

// Source is where a resolved config value came from.
type Source string

const (
	SourceFlag         Source = "flag"
	SourceEnv          Source = "env"
	SourceProject      Source = "project"
	SourceUser         Source = "user"
	SourceLegacyTS     Source = "legacy-typescript"
	SourceLegacyPython Source = "legacy-python"
	SourceLegacyJava   Source = "legacy-java"
	SourceDefault      Source = "default"
)

func (s Source) String() string {
	if s == "" {
		return "(unset)"
	}
	return string(s)
}
