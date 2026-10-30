#!/usr/bin/env python3
import base64
import json
import sys
import urllib.error
import urllib.parse
import urllib.request


REQUIRED_EVENTS = {"checkout.session.completed", "payment_intent.succeeded"}


def check_webhook(environment, expected_url, opener=urllib.request.urlopen):
    if environment.get("STRIPE_BILLING_ENABLED", "false").lower() not in {"1", "t", "true"}:
        return "Stripe billing disabled; webhook preflight skipped"

    stripe_key = environment.get("STRIPE_SECRET_KEY", "")
    signing_secret = environment.get("STRIPE_WEBHOOK_SIGNING_SECRET", "")
    if not stripe_key or not signing_secret:
        raise RuntimeError("Stripe billing is enabled but its secret key or webhook signing secret is missing")

    authorization = base64.b64encode((stripe_key + ":").encode()).decode()
    endpoints = []
    starting_after = ""
    while True:
        query = {"limit": "100"}
        if starting_after:
            query["starting_after"] = starting_after
        request = urllib.request.Request(
            "https://api.stripe.com/v1/webhook_endpoints?" + urllib.parse.urlencode(query),
            headers={
                "Authorization": "Basic " + authorization,
                "User-Agent": "Helix-Stripe-Webhook-Preflight/1.0",
            },
        )
        try:
            with opener(request, timeout=30) as response:
                page = json.load(response)
        except (urllib.error.URLError, json.JSONDecodeError, TimeoutError) as error:
            raise RuntimeError("failed to list Stripe webhook endpoints") from error

        if not isinstance(page, dict):
            raise RuntimeError("Stripe webhook endpoint response is invalid")
        data = page.get("data")
        if not isinstance(data, list):
            raise RuntimeError("Stripe webhook endpoint response is invalid")
        endpoints.extend(data)
        if not page.get("has_more"):
            break
        if not data or not data[-1].get("id"):
            raise RuntimeError("Stripe webhook endpoint pagination is invalid")
        starting_after = data[-1]["id"]

    matches = [endpoint for endpoint in endpoints if endpoint.get("url") == expected_url and endpoint.get("status") == "enabled"]
    if len(matches) != 1:
        raise RuntimeError(f"expected one enabled Stripe webhook endpoint for {expected_url}, found {len(matches)}")

    enabled_events_value = matches[0].get("enabled_events")
    if not isinstance(enabled_events_value, list):
        raise RuntimeError("Stripe webhook endpoint event subscriptions are invalid")
    enabled_events = set(enabled_events_value)
    missing = set() if "*" in enabled_events else REQUIRED_EVENTS - enabled_events
    if missing:
        raise RuntimeError("Stripe webhook endpoint is missing required events: " + ", ".join(sorted(missing)))
    return "Stripe webhook preflight passed"


def main():
    if len(sys.argv) != 2:
        raise RuntimeError("expected webhook URL argument")
    values = json.load(sys.stdin)
    if not isinstance(values, list) or any(not isinstance(value, str) for value in values):
        raise RuntimeError("container environment is invalid")
    environment = dict(value.split("=", 1) for value in values if "=" in value)
    print(check_webhook(environment, sys.argv[1]))


if __name__ == "__main__":
    try:
        main()
    except (RuntimeError, json.JSONDecodeError) as error:
        print(f"Stripe webhook preflight failed: {error}", file=sys.stderr)
        raise SystemExit(1)
