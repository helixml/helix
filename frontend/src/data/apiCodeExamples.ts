interface ApiCodeExample {
  language: string;
  label: string;
  code: (url: string, apiKey: string, model: string) => string;
}

export const API_CODE_EXAMPLES: ApiCodeExample[] = [
  {
    language: 'python',
    label: 'OpenAI Python',
    code: (url: string, apiKey: string, model: string) => `from openai import OpenAI

client = OpenAI(
  base_url="${url}/v1",
  api_key="${apiKey}",
)

# First API call with reasoning
response = client.chat.completions.create(
  model="${model}",
  messages=[
          {
            "role": "user",
            "content": "How many r's are in the word 'strawberry'?"
          }
        ],
  extra_body={"reasoning": {"enabled": True}}
)

# Extract the assistant message with reasoning_details
response = response.choices[0].message

# Preserve the assistant message with reasoning_details
messages = [
  {"role": "user", "content": "How many r's are in the word 'strawberry'?"},
  {
    "role": "assistant",
    "content": response.content,
    "reasoning_details": response.reasoning_details  # Pass back unmodified
  },
  {"role": "user", "content": "Are you sure? Think carefully."}
]

# Second API call - model continues reasoning from where it left off
response2 = client.chat.completions.create(
  model="${model}",
  messages=messages,
  extra_body={"reasoning": {"enabled": True}}
)`,
  },
  {
    language: 'python',
    label: 'Python',
    code: (url: string, apiKey: string, model: string) => `import requests
import json

# First API call with reasoning
response = requests.post(
  url="${url}/v1/chat/completions",
  headers={
    "Authorization": "Bearer ${apiKey}",
    "Content-Type": "application/json",
  },
  data=json.dumps({
    "model": "${model}",
    "messages": [
        {
          "role": "user",
          "content": "How many r's are in the word 'strawberry'?"
        }
      ],
    "reasoning": {"enabled": True}
  })
)

# Extract the assistant message with reasoning_details
response = response.json()
response = response['choices'][0]['message']

# Preserve the assistant message with reasoning_details
messages = [
  {"role": "user", "content": "How many r's are in the word 'strawberry'?"},
  {
    "role": "assistant",
    "content": response.get('content'),
    "reasoning_details": response.get('reasoning_details')  # Pass back unmodified
  },
  {"role": "user", "content": "Are you sure? Think carefully."}
]

# Second API call - model continues reasoning from where it left off
response2 = requests.post(
  url="${url}/v1/chat/completions",
  data=json.dumps({
    "model": "${model}",
    "messages": messages,  # Includes preserved reasoning_details
    "reasoning": {"enabled": True}
  })
)`,
  },
  {
    language: 'typescript',
    label: 'TypeScript',
    code: (url: string, apiKey: string, model: string) => `// First API call with reasoning
let response = await fetch("${url}/v1/chat/completions", {
  method: "POST",
  headers: {
    "Authorization": \`Bearer ${apiKey}\`,
    "Content-Type": "application/json"
  },
  body: JSON.stringify({
    "model": "${model}",
    "messages": [
      {
        "role": "user",
        "content": "How many r's are in the word 'strawberry'?"
      }
    ],
    "reasoning": {"enabled": true}
  })
});

// Extract the assistant message with reasoning_details and save it to the response variable
const result = await response.json();
response = result.choices[0].message;

// Preserve the assistant message with reasoning_details
const messages = [
  {
    role: 'user',
    content: "How many r's are in the word 'strawberry'?",
  },
  {
    role: 'assistant',
    content: response.content,
    reasoning_details: response.reasoning_details, // Pass back unmodified
  },
  {
    role: 'user',
    content: "Are you sure? Think carefully.",
  },
];

// Second API call - model continues reasoning from where it left off
const response2 = await fetch("${url}/v1/chat/completions", {
  method: "POST",
  headers: {
    "Authorization": \`Bearer ${apiKey}\`,
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    "model": "${model}",
    "messages": messages  // Includes preserved reasoning_details
  })
});`,
  },
  {
    language: 'typescript',
    label: 'OpenAI TypeScript',
    code: (url: string, apiKey: string, model: string) => `import OpenAI from 'openai';

const client = new OpenAI({
  baseURL: '${url}/v1',
  apiKey: '${apiKey}',
});

// First API call with reasoning
const apiResponse = await client.chat.completions.create({
  model: '${model}',
  messages: [
    {
      role: 'user' as const,
      content: "How many r's are in the word 'strawberry'?",
    },
  ],
  reasoning: { enabled: true }
});

// Extract the assistant message with reasoning_details
type ORChatMessage = (typeof apiResponse)['choices'][number]['message'] & {
  reasoning_details?: unknown;
};
const response = apiResponse.choices[0].message as ORChatMessage;

// Preserve the assistant message with reasoning_details
const messages = [
  {
    role: 'user' as const,
    content: "How many r's are in the word 'strawberry'?",
  },
  {
    role: 'assistant' as const,
    content: response.content,
    reasoning_details: response.reasoning_details, // Pass back unmodified
  },
  {
    role: 'user' as const,
    content: "Are you sure? Think carefully.",
  },
];

// Second API call - model continues reasoning from where it left off
const response2 = await client.chat.completions.create({
  model: '${model}',
  messages, // Includes preserved reasoning_details
});`,
  },
  {
    language: 'bash',
    label: 'Curl',
    code: (url: string, apiKey: string, model: string) => `curl ${url}/v1/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer ${apiKey}" \\
  -d '{
  "model": "${model}",
  "messages": [
    {
      "role": "user",
      "content": "How many rs are in the word strawberry?"
    }
  ],
  "reasoning": {
    "enabled": true
  }
}'`,
  },
];
