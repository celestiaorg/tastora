package internal

import (
	"regexp"
	"strings"
)

// CondenseHostName truncates the middle of the given name
// if it is 64 characters or longer.
//
// Without this helper, you may see an error like:
//
//	API error (500): failed to create shim: OCI runtime create failed: container_linux.go:380: starting container process caused: process_linux.go:545: container init caused: sethostname: invalid argument: unknown
func CondenseHostName(name string) string {
	if len(name) < 64 {
		return name
	}

	// I wanted to use ... as the middle separator,
	// but that causes resolution problems for other hosts.
	// Instead, use _._ which will be okay if there is a . on either end.
	return name[:30] + "_._" + name[len(name)-30:]
}

var validContainerCharsRE = regexp.MustCompile(`[^a-zA-Z0-9_.-]`)

// SanitizeDockerResourceName returns name with any
// invalid characters replaced with underscores.
// Subtests will include slashes, and there may be other
// invalid characters too.
func SanitizeDockerResourceName(name string) string {
	return validContainerCharsRE.ReplaceAllLiteralString(name, "_")
}

// ParseCommandLineArgs converts a slice of command line arguments into a map for easy lookup.
// Arguments in the format "--key=value" are stored as key -> value.
// Arguments in the format "-key=value" are stored as key -> value.
// Arguments without "=" are stored as key -> "" (empty string).
func ParseCommandLineArgs(args []string) map[string]string {
	result := make(map[string]string)
	for _, arg := range args {
		if strings.HasPrefix(arg, "--") {
			parsePrefix(arg, "--", result)
		} else if strings.HasPrefix(arg, "-") {
			parsePrefix(arg, "-", result)
		}
	}
	return result
}

// parsePrefix removes the given prefix from arg and splits it into key and value at the first "=".
func parsePrefix(arg, prefix string, result map[string]string) {
	arg = strings.TrimPrefix(arg, prefix)
	if parts := strings.SplitN(arg, "=", 2); len(parts) == 2 {
		result[parts[0]] = parts[1]
	} else {
		result[arg] = ""
	}
}
