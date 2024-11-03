interface CodeExample {
  language: string;
  label: string;
  code: (address: string, apiKey: string) => string;
}

export const CODE_EXAMPLES: CodeExample[] = [
  {
    language: 'javascript',
    label: 'Node.js',
    code: (address: string, apiKey: string) => `
const axios = require('axios');

async function chat() {
  try {
    const response = await axios.post('${address}/v1/chat/completions', {
      model: 'llama3:instruct',
      messages: [
        { role: 'user', content: 'Hello, how are you?' }
      ]
    }, {
      headers: {
        'Authorization': 'Bearer ${apiKey}',
        'Content-Type': 'application/json'
      }
    });

    console.log(response.data);
  } catch (error) {
    console.error('Error:', error);
  }
}

chat();
`,
  },
  {
    language: 'python',
    label: 'Python',
    code: (address: string, apiKey: string) => `
import requests

def chat():
    url = "${address}/v1/chat/completions"
    headers = {
        "Authorization": f"Bearer ${apiKey}",
        "Content-Type": "application/json"
    }
    payload = {
        "model": "llama3:instruct",
        "messages": [
            {"role": "user", "content": "Hello, how are you?"}
        ]
    }

    try:
        response = requests.post(url, json=payload, headers=headers)
        response.raise_for_status()
        print(response.json())
    except requests.exceptions.RequestException as e:
        print(f"Error: {e}")

if __name__ == "__main__":
    chat()
`,
  },
  {
    language: 'go',
    label: 'Go',
    code: (address: string, apiKey: string) => `
// Using https://github.com/sashabaranov/go-openai
package main

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

func main() {
	config := openai.DefaultConfig("${apiKey}")
	config.BaseURL = "${address}/v1"

	client := openai.NewClientWithConfig(config)

	resp, err := client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: "Hello, how are you?",
				},
			},
		},
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(resp)
}
`,
  },
]; 