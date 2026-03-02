package janitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type SlackRequestBody struct {
	Text string `json:"text"`
}

func sendSlackNotification(webhookURL string, message string) error {
	data := SlackRequestBody{
		Text: message,
	}
	slackBody, err := json.Marshal(data)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewBuffer(slackBody))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return err
	}
	if buf.String() != "ok" {
		return fmt.Errorf("slack webhook returned %d: %s", resp.StatusCode, buf.String())
	}

	return nil
}
