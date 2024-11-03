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
// Using https://www.npmjs.com/package/openai
import OpenAI from 'openai';

const client = new OpenAI({
  apiKey: "${apiKey}", // Usually this should be set in the environment
  baseURL: "${address}/v1",
});

async function main() {
  const chatCompletion = await client.chat.completions.create({
    messages: [{ role: 'user', content: 'Hello, how are you?' }],    
  });

  console.log(chatCompletion.choices[0].message.content);
}

main();
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
package main

import (
	"context"
	"fmt"

	openai "github.com/sashabaranov/go-openai"
)

func main() {
  // Configure to use the Helix API key
	config := openai.DefaultConfig("${apiKey}")

	// Configure to use the Helix API endpoint
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