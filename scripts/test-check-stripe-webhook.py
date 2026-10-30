#!/usr/bin/env python3
import importlib.util
import io
import json
import pathlib
import unittest
import urllib.error


path = pathlib.Path(__file__).with_name("check-stripe-webhook.py")
spec = importlib.util.spec_from_file_location("check_stripe_webhook", path)
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)


class Response(io.BytesIO):
    def __enter__(self):
        return self

    def __exit__(self, *_):
        self.close()


class StripeWebhookTest(unittest.TestCase):
    environment = {
        "STRIPE_BILLING_ENABLED": "true",
        "STRIPE_SECRET_KEY": "sk_test_secret",
        "STRIPE_WEBHOOK_SIGNING_SECRET": "whsec_test_secret",
    }
    expected_url = "https://app.helix.test/api/v1/stripe/webhook"

    def opener(self, pages):
        requests = []

        def open_request(request, timeout):
            self.assertEqual(30, timeout)
            self.assertNotIn("sk_test_secret", request.full_url)
            requests.append(request)
            page = pages[len(requests) - 1]
            if isinstance(page, Exception):
                raise page
            return Response(json.dumps(page).encode())

        return requests, open_request

    def endpoint(self, events=None, **fields):
        return {
            "id": "we_1",
            "url": self.expected_url,
            "status": "enabled",
            "enabled_events": events or ["checkout.session.completed", "payment_intent.succeeded"],
            **fields,
        }

    def check(self, pages, environment=None):
        requests, opener = self.opener(pages)
        result = checker.check_webhook(environment or self.environment, self.expected_url, opener)
        return result, requests

    def test_valid_endpoint(self):
        result, requests = self.check([{"data": [self.endpoint()], "has_more": False}])
        self.assertEqual("Stripe webhook preflight passed", result)
        self.assertTrue(requests[0].get_header("Authorization").startswith("Basic "))

    def test_wildcard_endpoint(self):
        result, _ = self.check([{"data": [self.endpoint(["*"])], "has_more": False}])
        self.assertEqual("Stripe webhook preflight passed", result)

    def test_pagination(self):
        result, requests = self.check([
            {"data": [{"id": "we_other", "url": "https://other.test", "status": "enabled"}], "has_more": True},
            {"data": [self.endpoint()], "has_more": False},
        ])
        self.assertEqual("Stripe webhook preflight passed", result)
        self.assertIn("starting_after=we_other", requests[1].full_url)

    def test_disabled_billing_skips_without_secrets(self):
        result, requests = self.check([], {"STRIPE_BILLING_ENABLED": "false"})
        self.assertEqual("Stripe billing disabled; webhook preflight skipped", result)
        self.assertEqual([], requests)

    def test_missing_required_event_fails(self):
        with self.assertRaisesRegex(RuntimeError, "checkout.session.completed"):
            self.check([{"data": [self.endpoint(["payment_intent.succeeded"])], "has_more": False}])

    def test_wrong_url_fails(self):
        with self.assertRaisesRegex(RuntimeError, "found 0"):
            self.check([{"data": [self.endpoint(url="https://other.test")], "has_more": False}])

    def test_multiple_matches_fail(self):
        with self.assertRaisesRegex(RuntimeError, "found 2"):
            self.check([{"data": [self.endpoint(), self.endpoint(id="we_2")], "has_more": False}])

    def test_api_failure_hides_credentials(self):
        with self.assertRaisesRegex(RuntimeError, "failed to list") as raised:
            self.check([urllib.error.URLError("contains sk_test_secret")])
        self.assertNotIn("sk_test_secret", str(raised.exception))


if __name__ == "__main__":
    unittest.main()
