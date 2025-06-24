# Smoke-Tests

This directory contains smoke tests that run hourly on the production cluster. But you can target
another cluster and run them manually as a basic acceptance test. They are basic, standard go tests
with a special tag to prevent them from running by default.

## Prerequisites

- go

## Things to Be Aware Of

1. These test user interaction, so they follow user flows like logging in and clicking through
   menus. 
2. Since the tests are intended to test production services, most tests target production URLs and
   builds. For example, the tests in `helix_cli_apply_test.go` download a copy of the `main` branch
   of helix. They do not use local files.

## Running Locally

First, export the following required environment variables:

```bash
export SERVER_URL=https://app.helix.ml # The server to run the tests against
export HELIX_USER=phil+smoketest@helix.ml # A user that has access to the server
export HELIX_PASSWORD=xxxxx-xxxxx-xxxxx # The user's actual login password
```

Optionally, you may also export the following optional environment variables:

```bash
export BROWSER_URL=http://localhost:7317 # URL of a remote chrome browser running in a Go Rod server
export SHOW_BROWSER=1 # If set, the tests will set headless to false and open a browser to watch the tests run
```

Then, run the tests:

```bash
go test -timeout 300s -tags=integration -v ./integration-test/smoke -count=1
```

You may also run a single test by providing the name of the test to go test, for example:

```bash
go test -timeout 300s -tags=integration -v ./integration-test/smoke -count=5 -run TestHelixCLITest
```

## Triggering a Run on Drone

To trigger a run on Drone, you can use the following curl command:

```bash
curl --request POST \
  --url https://drone.lukemarsden.net/api/repos/helixml/helix/cron/smoke-test-hourly \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer xxxxxxxxxxxxxxx'
```

Where `xxxxxxxxxxxxxxxx` is a valid Drone API key for the `helixml` organization.

## Triggering a Run on Drone on a Custom Branch

1. Create a new cron job in Drone called developer and pointing to your-branch-name .
2. Then run:

```bash
curl --request POST \
  --url https://drone.lukemarsden.net/api/repos/helixml/helix/cron/developer?branch=your-branch-name \
  --header 'Content-Type: application/json' \
  --header 'Authorization: Bearer xxxxxxxxxxxxxxx'
```

Where `xxxxxxxxxxxxxxxx` is a valid Drone API key for the `helixml` organization.
