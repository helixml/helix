package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
)

func loadFile(filePath string) (string, error) {
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func chatWithModel(fileContent string) ([]string, error) {
	url := "http://localhost:11434/api/chat"
	headers := map[string]string{"Content-Type": "application/json"}
	messages := []map[string]string{
		{"role": "system", "content": "You are an intelligent professor. You create question and answer pairs from given context for your students. Respond with an array of strict JSON 'question' & 'answer' pairs."},
		{"role": "user", "content": fmt.Sprintf("Here is your context:\n%s", fileContent)},
	}
	data := map[string]interface{}{
		"model":    "nous-hermes2-mixtral",
		"messages": messages,
		"stream":   false,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	response, err := http.Post(url, headers["Content-Type"], strings.NewReader(string(jsonData)))
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	var result []string
	scanner := bufio.NewScanner(response.Body)
	for scanner.Scan() {
		line := scanner.Text()
		var message map[string]interface{}
		err := json.Unmarshal([]byte(line), &message)
		if err != nil {
			return nil, err
		}
		result = append(result, message["message"].(map[string]interface{})["content"].(string))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func Query() {
	fileContent, err := loadFile("text.txt")
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	response, err := chatWithModel(fileContent)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	for _, message := range response {
		fmt.Println(message)
	}
}
