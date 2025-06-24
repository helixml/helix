# Using a custom RAG server with Helix

Helix allows for custom RAG servers to be used with the knowledge base. This example demonstrates how to setup a custom RAG server with Helix.

Custom RAG servers can help you with:
- Retrieving data from your internal database
- Indexing non-textual data such as images, CSVs, sqlite databases, etc.
- Trying out different RAG server implementations

## Prerequisites

- [Helix account](https://app.helix.ml/) or self-hosted controlplane
- Node.js
- npm
- [Webhook Relay account](https://webhookrelay.com/) for development. When running in production, we recommend exposing your server to Helix directly as a sidecar container similarly to how we expose our Llamaindex based default implementation.

## Setup

Initialize the example node.js project:

```bash
cd examples/custom_rag/nodejs
npm install
```

Start the node.js server:

```bash
node server.js
```

Open a Webhook Relay tunnel to your local node.js server:

```bash
relay connect localhost:5000
```

Garb your public URL from the output (`http://1rvyoove1fvtgxtm3t38yf.webrelay.io <----> http://localhost:5000`) and set it accordingly to the app.yaml:

```yaml
description: |
  A simple app that demonstrates how to setup Helix with knowledge from a
  custom RAG server
assistants:
- name: Helix
  description: Knowledge about cars
  knowledge:
  - name: cars
    rag_settings:
      disable_chunking: true # Pass the entire document to the RAG server
      index_url: http://1rvyoove1fvtgxtm3t38yf.webrelay.io/api/index # <- replace with your domain
      query_url: http://1rvyoove1fvtgxtm3t38yf.webrelay.io/api/query # <- replace with your domain
    source:
      web:
        urls:
        - https://gist.githubusercontent.com/rusenask/d7d12da5bf8dd11a512e2f8143a4bd84/raw/bbf65a70aad34057b5595cb2aeaa8cf0c7d0277d/cars
```

## Creating the Helix app with a custom RAG server:

Login to the Helix CLI by copying he "CLI login" command from https://app.helix.ml/account.

Once you are ready, create the Helix app:

```bash
helix apply -f examples/custom_rag/nodejs/app.yaml
```

Running this command will create an app and return the ID such as `app_01j5r0cm5s1g7yjfjbpsjnknrj`, use this ID later to test the app.

In a few seconds you should see Helix indexing the knowledge:

```
 node server.js
Server running on http://0.0.0.0:5000
data_entity_id: kno_01j5r0cm5wjnts3s7rtpwc5txt
content: Karolis has a green car
Luke has a blue car
Kai has a red car
```

## Testing the app

You can test the app by running the following command:

```bash
curl --request POST \
  --url http://localhost:8080/api/v1/sessions/chat \
  --header 'Authorization: Bearer hl-XXX' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "meta-llama/Meta-Llama-3.1-8B-Instruct-Turbo",
    "session_id": "",
    "stream": false,
    "app_id": "app_01j5r0cm5s1g7yjfjbpsjnknrj",    
    "messages": [      
      { "role": "user", "content": {"parts": ["what is the color of kai'\''s car?"]} }
    ]
  }'
```

Response should be:

```json
{
  "id": "ses_01j5r0ejx60vq8ne0a543mmvwn",
  "object": "chat.completion",
  "created": 1724161412,
  "model": "meta-llama/meta-llama-3.1-8b-instruct-turbo",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "According to [DOC_ID:1], the color of Kai's car is red."
      },
      "finish_reason": "eos"
    }
  ],
  "usage": {
    "prompt_tokens": 172,
    "completion_tokens": 18,
    "total_tokens": 190
  },
  "system_fingerprint": ""
}
```