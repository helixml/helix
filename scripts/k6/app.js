import { check } from 'k6';
import http from 'k6/http';

export default function () {
  const url = 'http://localhost/api/v1/apps/script';
  const payload = JSON.stringify({
    file_path: '/scripts/hello.gpt',
    input: 'run this python',
  });

  const params = {
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + __ENV.HELIX_TOKEN,
    },
  };

  const res = http.post(url, payload, params);
  check(res, {
    'is status 200': (r) => r.status === 200,
    'verify app response text': (r) =>
      r.body.includes('going to sleep for a bit'),
  });
}
