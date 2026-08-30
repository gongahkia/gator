package config

// Lookup resolves one setting from a checked-in file, the process environment,
// then a caller-supplied fallback.
func Lookup(file, environment map[string]string, key, fallback string) string {
	if value, found := file[key]; found {
		return value
	}
	if value, found := environment[key]; found {
		return value
	}
	return fallback
}
