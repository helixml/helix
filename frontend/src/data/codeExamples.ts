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
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

type Message struct {
    Role    string \`json:"role"\`
    Content string \`json:"content"\`
}

type ChatRequest struct {
    Model    string    \`json:"model"\`
    Messages []Message \`json:"messages"\`
}

func main() {
    url := "${address}/v1/chat/completions"
    
    request := ChatRequest{
        Model: "llama3:instruct",
        Messages: []Message{
            {
                Role:    "user",
                Content: "Hello, how are you?",
            },
        },
    }
    
    jsonData, err := json.Marshal(request)
    if err != nil {
        fmt.Printf("Error marshaling JSON: %v\\n", err)
        return
    }

    req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
    if err != nil {
        fmt.Printf("Error creating request: %v\\n", err)
        return
    }

    req.Header.Set("Authorization", "Bearer ${apiKey}")
    req.Header.Set("Content-Type", "application/json")

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        fmt.Printf("Error making request: %v\\n", err)
        return
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    fmt.Println(result)
}
`,
  },
]; 