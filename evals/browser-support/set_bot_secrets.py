#!/usr/bin/env python3
"""Store the mock-system credentials as project secrets of a bot and bind them
to the bot for get_secret.

    python3 set_bot_secrets.py <bot-id> [<bot-id> ...]
"""
import sys

import run_eval as r

SECRETS = {
    "CRM_PASSWORD": "Crm!2026-Acme",
    "BILLING_PASSWORD": "B1llPro#Secure",
    "BILLING_SECURITY_ANSWER": "Kaunas",
    "HELPDESK_PASSWORD": "Help-Desk-77",
    "SAUCEDEMO_PASSWORD": "secret_sauce",
}

for bot in sys.argv[1:]:
    project_id = r.api("GET", f"/orgs/{r.ORG}/bots/{bot}")["project_id"]
    existing = {s["name"]: s for s in (r.api("GET", f"/secrets?project_id={project_id}") or []) if s.get("project_id") == project_id}
    for name, value in SECRETS.items():
        sec = existing.get(name) or r.api("POST", "/secrets", {"name": name, "value": value, "project_id": project_id})
        r.api("PUT", f"/orgs/{r.ORG}/bots/{bot}/secrets/{name}",
              {"source_kind": "helix_secret", "secret_id": sec["id"], "description": f"{name} for the mock systems"})
    print(bot, "secrets bound:", ", ".join(SECRETS))
