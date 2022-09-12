package http_middleware

// LogOption is an option for a request logger.
type LogOption func(*options)

// RequestFilter returns false if the request should be filtered out and true otherwise
type RequestFilter func(url string) bool

// ExcludeURLs returns a RequestFilter that only logs requests for URLs in the urls parameter
func ExcludeURLs(urls ...string) RequestFilter {
	return func(url string) bool {
		for _, u := range urls {
			if url == u {
				return false
			}
		}
		return true
	}
}

func WithLogBody(b bool) LogOption {
	return func(o *options) {
		o.logBody = b
	}
}

func SkipURL(urls ...string) LogOption {
	return func(o *options) {
		o.skippedURLs = append(o.skippedURLs, urls...)
	}
}

var defaultSkippedURLs = []string{
	"/metrics",
	"/healthz",
	"/ok",
	"/shutdownz",
	"/version",
	"/",
}

type options struct {
	filters      []RequestFilter
	headersToLog []string
	logBody      bool
	skippedURLs  []string
}
