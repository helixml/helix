import { check, sleep } from "k6";
import http from "k6/http";


const test_data = [
  "What is the capital of Australia?",
  "How does a blockchain work?",
  "Who was the first president of the United States?",
  "What are the stages of the water cycle?",
  "How do you solve a quadratic equation?",
  "What is the theory of evolution by natural selection?",
  "What causes earthquakes?",
  "What is the difference between renewable and non-renewable energy sources?",
  "How do vaccines work?",
  "What is the significance of the Magna Carta?",
]

// Simulated user behavior.
// To run the script:
// k6 run --vus 10 --duration 300s scripts/k6/openai.js
export default function () {
  let data = {
    "model": "llama3.1:8b-instruct-q8_0",
    "messages": [
      {
        "role": "user",
        "content": test_data[Math.floor(Math.random() * test_data.length)] + " Your answer must be shorter than 30 words."
      },
    ],
    stream: false,
  };

  let res = http.post("https://app.helix.ml/v1/chat/completions", JSON.stringify(data), {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + __ENV.HELIX_API_KEY,
    },
  });

  // Validate response status
  check(res, { "status was 200": (r) => r.status == 200 });
  sleep(1);
}