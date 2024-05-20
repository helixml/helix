# Testing

## Prerequisites

- Install k6 (https://k6.io/docs/get-started/installation/). 
- Get your Helix API key

## Testing Apps

First, set your Helix API key as an environment variable, e.g.:

```
export HELIX_TOKEN=hl-4oYdt9bANk1XETZ0tb-OizzP2qELLu8XS9Bjz2zIFrs=
```

By default you can load this https://github.com/helixml/run-python-helix-app app to test.

```
k6 run --vus 10 --duration 30s scripts/k6/app.js
```