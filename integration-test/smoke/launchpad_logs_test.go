//go:build launchpad

package smoke

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod"
	"github.com/helixml/helix/integration-test/smoke/helper"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const (
	uniqueWord = "ungreased"
)

type LaunchpadLogsSuite struct {
	suite.Suite
	launchpadClient *LaunchpadClient
	deployment      *LaunchpadDeployment
	browser         *rod.Browser
}

func TestLaunchpadLogsSuite(t *testing.T) {
	suite.Run(t, new(LaunchpadLogsSuite))
}

func (s *LaunchpadLogsSuite) SetupSuite() {
	s.T().Log("SetupSuite")
	s.launchpadClient = &LaunchpadClient{
		APIKey: os.Getenv("LAUNCHPAD_API_KEY"),
	}

	// Ensure the API key is set
	if s.launchpadClient.APIKey == "" {
		s.T().Fatalf("LAUNCHPAD_API_KEY is not set")
	}

	// Ensure there are no launchpad instances running
	deployments, err := s.launchpadClient.ListDeployments()
	if err != nil {
		s.T().Fatalf("Failed to get launchpad deployments: %v", err)
	}

	for _, deployment := range deployments {
		helper.LogStep(s.T(), fmt.Sprintf("Deleting deployment %s", deployment.ID))
		err := s.launchpadClient.DeleteDeployment(deployment.ID)
		if err != nil {
			s.T().Fatalf("Failed to delete launchpad deployment: %v", err)
		}
	}

	// Create a test instance
	instance, err := s.launchpadClient.CreateInstance("smoke-test")
	if err != nil {
		s.T().Fatalf("Failed to create launchpad instance: %v", err)
	}

	s.deployment = instance
	helper.LogStep(s.T(), fmt.Sprintf("Created deployment %s", instance.ID))

	// Wait for the instance to be ready
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

LOOP:
	for {
		select {
		case <-timeoutCtx.Done():
			s.T().Fatalf("Instance failed to start")
		case <-time.After(100 * time.Millisecond):
			deployments, err := s.launchpadClient.ListDeployments()
			if err != nil {
				s.T().Fatalf("Failed to get launchpad deployment: %v", err)
			}
			for _, deployment := range deployments {
				if deployment.ID == s.deployment.ID {
					s.deployment = &deployment
					if deployment.Status == "running" {
						helper.LogStep(s.T(), "Instance started")
						break LOOP
					}
				}
			}
		}
	}

	ctx := helper.CreateContext(s.T())
	s.browser = createBrowser(ctx)
}

func (s *LaunchpadLogsSuite) TearDownSuite() {
	s.T().Log("TearDownSuite")
	s.browser.MustClose()
	helper.LogStep(s.T(), fmt.Sprintf("Deleting deployment %s", s.deployment.ID))
	err := s.launchpadClient.DeleteDeployment(s.deployment.ID)
	if err != nil {
		s.T().Fatalf("Failed to delete launchpad deployment: %v", err)
	}
}

func (s *LaunchpadLogsSuite) TestLaunchpad() {
	// Convert the IPv4 address to dash separated format
	ipv4 := strings.ReplaceAll(s.deployment.IPv4, ".", "-")
	url := fmt.Sprintf("https://%s.helix.cluster.world/", ipv4)
	helper.LogStep(s.T(), fmt.Sprintf("Connecting to %s", url))

	// Wait for the instance to return a 200 response
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

LOOP:
	for {
		select {
		case <-timeoutCtx.Done():
			s.T().Fatalf("Instance failed to start")
		case <-time.After(100 * time.Millisecond):
			resp, err := http.Get(url)
			if err != nil {
				continue
			}
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				helper.LogStep(s.T(), "Instance started")
				break LOOP
			}
		}
	}

	// Connect a browser to the instance
	page := s.browser.MustPage(url)
	page.MustWaitLoad()

	// Register a new user
	helper.RegisterNewUser(s.T(), page)

	// Send a message
	helper.SendMessage(s.T(), page, fmt.Sprintf("I want you to say the word %s, you must say %s", uniqueWord, uniqueWord))

	// Create a temporary directory
	tempDir, err := os.MkdirTemp("", "helix-logs-*")
	require.NoError(s.T(), err, "creating temp dir should succeed")
	defer os.RemoveAll(tempDir)

	// Download the logs
	zipFile, err := s.launchpadClient.GetLogs(s.deployment.ID, tempDir)
	require.NoError(s.T(), err, "getting logs should succeed")

	// Open the zip file
	reader, err := zip.OpenReader(zipFile)
	require.NoError(s.T(), err, "opening zip file should succeed")
	defer reader.Close()

	// Extract each file
	for _, file := range reader.File {
		// Open the file inside the zip
		rc, err := file.Open()
		require.NoError(s.T(), err, "opening file in zip should succeed")

		// Create the file path
		path := filepath.Join(tempDir, file.Name)

		// Create the directory structure
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			rc.Close()
			require.NoError(s.T(), err, "creating directories should succeed")
		}

		// Create the file
		outFile, err := os.Create(path)
		require.NoError(s.T(), err, "creating output file should succeed")

		// Copy the contents
		_, err = io.Copy(outFile, rc)
		outFile.Close()
		rc.Close()
		require.NoError(s.T(), err, "copying file contents should succeed")
	}

	// List the files in the temp dir
	files, err := os.ReadDir(tempDir)
	require.NoError(s.T(), err, "reading temp dir should succeed")

	// Cat the files in the temp dir and look for the string the unique word
	found := false
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(tempDir, file.Name()))
		require.NoError(s.T(), err, "reading file should succeed")
		if strings.Contains(string(content), uniqueWord) {
			s.T().Logf("Found '%s' in file: %s", uniqueWord, file.Name())
			found = true
		}
	}
	if found {
		helper.LogAndFail(s.T(), fmt.Sprintf("Found the word %s in the logs", uniqueWord))
	} else {
		helper.LogAndPass(s.T(), fmt.Sprintf("Did not find the word %s in the logs", uniqueWord))
	}
}

type LaunchpadClient struct {
	APIKey string
}

type LaunchpadDeployment struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	UserID             string    `json:"user_id"`
	ServerID           string    `json:"server_id"`
	Status             string    `json:"status"`
	IPv4               string    `json:"ipv4"`
	SSHPrivateKey      string    `json:"ssh_private_key"`
	SSHPublicKey       string    `json:"ssh_public_key"`
	Provider           string    `json:"provider"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	TimeoutMins        *int      `json:"timeout_mins"`
	DeploymentConfigID string    `json:"deployment_config_id"`
}

func (c *LaunchpadClient) ListDeployments() ([]LaunchpadDeployment, error) {
	url := "https://deploy.helix.ml/api/deployments"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("LAUNCHPAD_API_KEY")))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list deployments: %s", res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var deployments []LaunchpadDeployment
	err = json.Unmarshal(body, &deployments)
	if err != nil {
		return nil, err
	}

	return deployments, nil
}

func (c *LaunchpadClient) DeleteDeployment(deploymentID string) error {
	url := fmt.Sprintf("https://deploy.helix.ml/api/deployments/%s", deploymentID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", os.Getenv("LAUNCHPAD_API_KEY")))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete deployment: %s", res.Status)
	}

	return nil
}

type CreateInstanceRequest struct {
	ConfigID    string `json:"configId"`
	Name        string `json:"name"`
	TimeoutMins *int   `json:"timeoutMins"`
}

func (c *LaunchpadClient) CreateInstance(name string) (*LaunchpadDeployment, error) {
	request := CreateInstanceRequest{
		ConfigID:    "trial_vm_hetzner",
		Name:        name,
		TimeoutMins: nil,
	}

	jsonBody, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}

	url := "https://deploy.helix.ml/api/deployments"
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	req.Header.Add("Content-Type", "application/json")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create instance: %s", res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var deployment LaunchpadDeployment
	err = json.Unmarshal(body, &deployment)
	if err != nil {
		return nil, err
	}

	return &deployment, nil
}

// GetLogs downloads a zip file of the logs to a temporary directory and returns the path to the zip file
func (c *LaunchpadClient) GetLogs(deploymentID, tempDir string) (string, error) {
	url := fmt.Sprintf("https://deploy.helix.ml/api/deployments/%s/logs", deploymentID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", c.APIKey))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get logs: %s", res.Status)
	}

	// We want to extract the filename from the content-disposition header
	contentDisposition := res.Header.Get("content-disposition")
	filename := strings.Split(contentDisposition, "=")[1]

	// Check that it is a zip file
	if !strings.HasSuffix(filename, ".zip") {
		return "", fmt.Errorf("logs are not a zip file")
	}

	// Download the file to a temporary directory
	tempFile, err := os.Create(filepath.Join(tempDir, filename))
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	// Copy the response body to the file
	_, err = io.Copy(tempFile, res.Body)
	if err != nil {
		return "", err
	}

	return tempFile.Name(), nil
}
