import http from "k6/http";
import { check, sleep } from "k6";


// Simulated user behavior.
// To run the script:
// k6 run --vus 10 --duration 300s scripts/k6/openai.js
export default function () {
  let data = {
    model: "helix-3.5",
    stream: false,
    messages: [
      {role: "user", content: "why is the sky blue?"}
    ],
  };

  let res = http.post("http://localhost:8080/v1/chat/completions", JSON.stringify(data), {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + __ENV.HELIX_API_KEY,
    },
  });

  // Validate response status
  check(res, { "status was 200": (r) => r.status == 200 });
  sleep(1);
}
