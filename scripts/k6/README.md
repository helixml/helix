# Testing

## Prerequisites

Install k6 (https://k6.io/docs/get-started/installation/). 


## Testing Apps

Modify scripts/k6/app.js with your own API key and app ID. By default you can load this https://github.com/helixml/run-python-helix-app app to test.

```
k6 run --vus 10 --duration 30s scripts/k6/app.js
```