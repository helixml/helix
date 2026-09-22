package server

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFilestoreUploadDestination(t *testing.T) {
	tests := []struct {
		name             string
		requestPath      string
		uploadedFilename string
		want             string
		wantError        bool
	}{
		{
			name:             "directory path appends multipart filename",
			requestPath:      "engagements/prj_1/retests",
			uploadedFilename: "retest_1.json",
			want:             "engagements/prj_1/retests/retest_1.json",
		},
		{
			name:             "matching full path remains exact",
			requestPath:      "engagements/prj_1/retests/retest_1.json",
			uploadedFilename: "retest_1.json",
			want:             "engagements/prj_1/retests/retest_1.json",
		},
		{
			name:             "suffix collision is still a directory",
			requestPath:      "engagements/prj_1/my-retest_1.json",
			uploadedFilename: "retest_1.json",
			want:             "engagements/prj_1/my-retest_1.json/retest_1.json",
		},
		{
			name:             "trailing slash disambiguates matching directory name",
			requestPath:      "engagements/prj_1/retest_1.json/",
			uploadedFilename: "retest_1.json",
			want:             "engagements/prj_1/retest_1.json/retest_1.json",
		},
		{
			name:             "empty path writes at user root",
			requestPath:      "",
			uploadedFilename: "receipt.json",
			want:             "receipt.json",
		},
		{
			name:             "parent traversal is rejected",
			requestPath:      "../../users/other-user",
			uploadedFilename: "receipt.json",
			wantError:        true,
		},
		{
			name:             "empty multipart filename is rejected",
			requestPath:      "engagements/prj_1/retests",
			uploadedFilename: "",
			wantError:        true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := filestoreUploadDestination(test.requestPath, test.uploadedFilename)
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		})
	}
}

func TestFilestoreAppUploadDestination(t *testing.T) {
	tests := []struct {
		name        string
		requestPath string
		appID       string
		want        string
		wantError   bool
	}{
		{
			name:        "app directory path appends multipart filename",
			requestPath: "apps/app_1/documents",
			appID:       "app_1",
			want:        "documents/report.txt",
		},
		{
			name:        "app exact full path remains exact",
			requestPath: "apps/app_1/documents/report.txt",
			appID:       "app_1",
			want:        "documents/report.txt",
		},
		{
			name:        "app root writes at app root",
			requestPath: "apps/app_1",
			appID:       "app_1",
			want:        "report.txt",
		},
		{
			name:        "different app prefix is rejected",
			requestPath: "apps/app_10/documents",
			appID:       "app_1",
			wantError:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := filestoreAppUploadDestination(test.requestPath, test.appID, "report.txt")
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.want, filepath.ToSlash(got))
		})
	}
}
