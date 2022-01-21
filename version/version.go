package version

// GitSHA is set at build time.
var GitSHA string

// Tag is set at build time.
var Tag string

func Version() string {
	if Tag != "" {
		return Tag
	} else if GitSHA != "" {
		return GitSHA
	}
	return "unknown"
}
