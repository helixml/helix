package janitor

import (
	"bytes"
	"encoding/json"
	"net/http"
)

type SlackRequestBody struct {
	Text string `json:"text"`
}

func sendSlackNotification(webhookUrl string, message string) error {
	data := SlackRequestBody{
		Text: message,
	}
	slackBody, err := json.Marshal(data)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, webhookUrl, bytes.NewBuffer(slackBody))
	if err != nil {
		return err
	}
	req.Header.Add("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	if buf.String() != "ok" {
		return err
	}

	return nil
}
