module gitlab.com/gitlab-org/gitlab-shell

go 1.13

require (
	github.com/mattn/go-shellwords v1.0.11
	github.com/otiai10/copy v1.4.2
	github.com/sirupsen/logrus v1.7.0
	github.com/stretchr/testify v1.6.1
	gitlab.com/gitlab-org/gitaly v1.68.0
	gitlab.com/gitlab-org/labkit v1.3.0
	golang.org/x/crypto v0.0.0-20201221181555-eec23a3978ad
	golang.org/x/sync v0.0.0-20201207232520-09787c993a3a
	google.golang.org/grpc v1.35.0
	gopkg.in/yaml.v2 v2.4.0
)

// go get tries to enforce semantic version compatibility via module paths.
// We can't upgrade to Gitaly v13.x.x from v1.x.x without using a manual override.
// See https://gitlab.com/gitlab-org/gitaly/-/issues/3177 for more details.
replace gitlab.com/gitlab-org/gitaly => gitlab.com/gitlab-org/gitaly v0.0.0-20201001041716-3f5e218def93
