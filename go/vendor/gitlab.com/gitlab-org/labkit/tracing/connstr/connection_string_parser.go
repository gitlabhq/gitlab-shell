package connstr

import (
	"fmt"
	"net/url"
	"regexp"
)

// Connection strings:
// * opentracing://jaeger
// * opentracing://datadog
// * opentracing://lightstep?access_key=12345

var errInvalidConnection = fmt.Errorf("invalid connection string")

// Parse parses a opentracing connection string into a driverName and options map.
func Parse(connectionString string) (driverName string, options map[string]string, err error) {
	if connectionString == "" {
		return "", nil, errInvalidConnection
	}

	URL, err := url.Parse(connectionString)
	if err != nil {
		return "", nil, errInvalidConnection
	}

	if URL.Scheme != "opentracing" {
		return "", nil, errInvalidConnection
	}

	driverName = URL.Host
	if driverName == "" {
		return "", nil, errInvalidConnection
	}

	// Connection strings should not have a path
	if URL.Path != "" {
		return "", nil, errInvalidConnection
	}

	matched, err := regexp.MatchString("^[a-z0-9_]+$", driverName)
	if err != nil || !matched {
		return "", nil, errInvalidConnection
	}

	query := URL.Query()
	driverName = URL.Host
	options = make(map[string]string, len(query))

	for k := range query {
		options[k] = query.Get(k)
	}

	return driverName, options, nil
}
