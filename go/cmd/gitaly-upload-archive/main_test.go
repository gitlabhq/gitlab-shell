package main

import (
	"fmt"
	"strings"
	"testing"

	pb "gitlab.com/gitlab-org/gitaly-proto/go/gitalypb"
)

var testGitalyAddress = "unix:gitaly.socket"

func TestUploadArchiveSuccess(t *testing.T) {
	testRelativePath := "myrepo.git"
	requestJSON := fmt.Sprintf(`{"repository":{"relative_path":"%s"}}`, testRelativePath)
	mockHandler := func(gitalyAddress string, request *pb.SSHUploadArchiveRequest) (int32, error) {
		if gitalyAddress != testGitalyAddress {
			t.Fatalf("Expected gitaly address %s got %v", testGitalyAddress, gitalyAddress)
		}
		if relativePath := request.Repository.RelativePath; relativePath != testRelativePath {
			t.Fatalf("Expected repository with relative path %s got %v", testRelativePath, request)
		}
		return 0, nil
	}

	code, err := uploadArchive(mockHandler, []string{"git-upload-archive", testGitalyAddress, requestJSON})

	if err != nil {
		t.Fatal(err)
	}

	if code != 0 {
		t.Fatalf("Expected exit code 0, got %v", code)
	}
}

func TestUploadArchiveFailure(t *testing.T) {
	mockHandler := func(_ string, _ *pb.SSHUploadArchiveRequest) (int32, error) {
		t.Fatal("Expected handler not to be called")

		return 0, nil
	}

	tests := []struct {
		desc string
		args []string
		err  string
	}{
		{
			desc: "With an invalid request json",
			args: []string{"git-upload-archive", testGitalyAddress, "hello"},
			err:  "unmarshaling request json failed",
		},
		{
			desc: "With an invalid argument count",
			args: []string{"git-upload-archive", testGitalyAddress, "{}", "extra arg"},
			err:  "wrong number of arguments: expected 2 arguments",
		},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			_, err := uploadArchive(mockHandler, test.args)

			if !strings.Contains(err.Error(), test.err) {
				t.Fatalf("Expected error %v, got %v", test.err, err)
			}
		})
	}
}
