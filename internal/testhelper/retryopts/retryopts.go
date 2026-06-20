// Package retryopts provides test helpers for configuring fast HTTP retry
// behavior in tests. It is a separate package from testhelper to avoid an
// import cycle: the client package's own tests import testhelper, so testhelper
// must not import client.
package retryopts

import (
	"time"

	"gitlab.com/gitlab-org/gitlab-shell/v14/client"
)

// FastRetryOpts returns HTTPClientOpts with near-zero retry delays so that
// tests exercising error paths (e.g. 500 responses) complete in milliseconds
// instead of seconds.
func FastRetryOpts() []client.HTTPClientOpt {
	return []client.HTTPClientOpt{
		client.WithHTTPRetryOpts(time.Millisecond, time.Millisecond, 2),
	}
}
