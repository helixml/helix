#!/usr/bin/env python3
"""Mock legacy back-office systems for browser-agent support evals.

Three deliberately awkward web apps served from one process:

  /crm/       Acme CRM 7.2       CSRF-protected login, paginated customer list,
                                 search, detail pages.
  /billing/   BillPro Enterprise two-step login + security question, a
                                 maintenance interstitial, balance loaded by JS.
  /helpdesk/  HelpDesk Classic   login needs the right domain in a dropdown,
                                 ticket history rendered inside an iframe.

Data is generated from a fixed seed, so ground truth is stable. Every request
is appended to access.log (JSON lines) for per-run tool-usage analysis.

    python3 mock_systems.py --port 8765 [--dump-truth truth.json]
"""

import argparse
import html
import json
import re
import random
import secrets
import threading
import time
from datetime import date, timedelta
from http.cookies import SimpleCookie
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.parse import parse_qs, urlparse

CREDENTIALS = {
    "crm": {"username": "support.agent", "password": "Crm!2026-Acme"},
    "billing": {
        "username": "svc_support",
        "password": "B1llPro#Secure",
        "security_answer": "Kaunas",
    },
    "helpdesk": {"username": "jdoe", "password": "Help-Desk-77", "domain": "CORP"},
}

FIRST = ["Northwind", "Globex", "Initech", "Umbrella", "Stark", "Wayne", "Cyberdyne",
         "Soylent", "Hooli", "Vandelay", "Wonka", "Tyrell", "Massive", "Aperture",
         "Oscorp", "Gringotts", "Pied Piper", "Dunder", "Prestige", "Monarch",
         "Sterling", "Bluth", "Acme", "Duff", "Krusty", "Nakatomi", "Weyland",
         "Rekall", "Virtucon", "Zorg", "Octan", "Buy n Large", "Ollivander",
         "Paper Street", "Blue Sun", "Kerbal", "Black Mesa", "Abstergo", "Omni",
         "Sirius", "Gekko", "Ecorp", "Lunar", "Mooby", "Yoyodyne", "Frobozz",
         "Spacely", "Cogswell", "Stay Puft", "Genco", "Bubba Gump", "Los Pollos",
         "Mom's", "Primatech", "Sabre", "Wernham", "Veidt", "Ghostwood", "Brawndo",
         "Tricell", "InGen", "Biffco", "Gideon", "Axiom", "Lacuna", "Nexus",
         "Polymer", "Quartz", "Riverside", "Summit", "Tidewater", "Zenith"]
SUFFIX = ["Traders", "Corporation", "Industries", "Holdings", "Logistics", "Labs",
          "Systems", "Group", "Partners", "Foods"]
MANAGERS = ["Priya Natarajan", "Tomas Kowalski", "Ines Duarte", "Marcus Webb",
            "Aiko Tanaka", "Lukas Petrauskas"]
TIERS = ["Bronze", "Silver", "Gold", "Platinum"]
TICKET_SUBJECTS = ["Cannot export invoices to CSV", "Login loop after password reset",
                   "Duplicate charge on last invoice", "API rate limit errors",
                   "Report shows wrong currency", "User seat count incorrect",
                   "SSO certificate expired", "Slow dashboard load",
                   "Missing shipment notifications", "Request to change billing contact"]
AGENTS = ["R. Silva", "K. Osei", "M. Lindqvist", "D. Okafor"]


def build_data(seed=20260924):
    rng = random.Random(seed)
    names = []
    for i, first in enumerate(FIRST):
        names.append(f"{first} {SUFFIX[i % len(SUFFIX)]}")
    customers = []
    for i, name in enumerate(names):
        cid = 1001 + i
        customers.append({
            "id": cid,
            "name": name,
            "tier": rng.choices(TIERS, weights=[4, 4, 3, 2])[0],
            "manager": rng.choice(MANAGERS),
            "renewal": (date(2026, 10, 1) + timedelta(days=rng.randint(0, 540))).isoformat(),
            "billing_account": f"BA-{10400 + i * 7}",
            "phone": f"+1-555-{rng.randint(100, 999)}-{rng.randint(1000, 9999)}",
            "seats": rng.randint(5, 900),
        })
    invoices = {}
    for c in customers:
        rows = []
        for n in range(rng.randint(3, 9)):
            issued = date(2026, 1, 5) + timedelta(days=30 * n + rng.randint(0, 4))
            amount = round(rng.uniform(250, 9800), 2)
            status = rng.choices(["Paid", "Open", "Overdue"], weights=[6, 2, 2])[0]
            rows.append({
                "number": f"INV-{c['id']}-{n + 1:02d}",
                "issued": issued.isoformat(),
                "due": (issued + timedelta(days=30)).isoformat(),
                "amount": amount,
                "status": status,
            })
        invoices[c["billing_account"]] = rows
    tickets = []
    tid = 5001
    for c in customers:
        for _ in range(rng.randint(0, 3)):
            status = rng.choices(["Open", "Pending Customer", "Closed"], weights=[3, 2, 4])[0]
            opened = date(2026, 6, 1) + timedelta(days=rng.randint(0, 100))
            history = [{"at": opened.isoformat(), "status": "New", "by": "system"}]
            mid = rng.choice(["In Progress", "Escalated", "Pending Customer"])
            history.append({"at": (opened + timedelta(days=1)).isoformat(), "status": mid,
                            "by": rng.choice(AGENTS)})
            if status == "Closed":
                history.append({"at": (opened + timedelta(days=rng.randint(2, 9))).isoformat(),
                                "status": "Closed", "by": rng.choice(AGENTS)})
            elif status != mid:
                history.append({"at": (opened + timedelta(days=2)).isoformat(),
                                "status": status, "by": rng.choice(AGENTS)})
            tickets.append({
                "id": tid,
                "customer_id": c["id"],
                "subject": rng.choice(TICKET_SUBJECTS),
                "priority": rng.choice(["P1 - Critical", "P2 - High", "P3 - Normal", "P4 - Low"]),
                "status": status,
                "opened": opened.isoformat(),
                "history": history,
            })
            tid += 1
    return customers, invoices, tickets


CUSTOMERS, INVOICES, TICKETS = build_data()
CUST_BY_ID = {c["id"]: c for c in CUSTOMERS}
CUST_BY_ACCT = {c["billing_account"]: c for c in CUSTOMERS}
TICKET_BY_ID = {t["id"]: t for t in TICKETS}
SESSIONS = {}  # sid -> {"system": ..., "stage": ...}
LOCK = threading.Lock()
LOG_PATH = "access.log"
LINK_RE = re.compile(r"""(["'=])/(crm|billing|helpdesk)/""")

LEGACY_CSS = """
body{font-family:Tahoma,Verdana,sans-serif;font-size:11px;background:#d4d0c8;margin:0}
.hdr{background:linear-gradient(#0a246a,#3a6ea5);color:#fff;padding:4px 8px;font-weight:bold}
.box{background:#fff;border:2px inset #999;margin:8px;padding:8px}
table.grid{border-collapse:collapse;width:100%}
table.grid td,table.grid th{border:1px solid #808080;padding:2px 4px;font-size:11px}
table.grid th{background:#ece9d8}
.nav a{margin-right:10px;color:#fff}
.err{color:#c00;font-weight:bold}
"""


def outstanding(acct):
    return round(sum(i["amount"] for i in INVOICES[acct] if i["status"] != "Paid"), 2)


def page(title, body, system_title, nav=""):
    return f"""<!DOCTYPE html><html><head><title>{html.escape(title)}</title>
<style>{LEGACY_CSS}</style></head><body>
<div class="hdr">{system_title} <span class="nav">{nav}</span></div>
<div class="box">{body}</div>
<div style="margin:8px;color:#555">(c) 1999-2026 — Unauthorised access prohibited. Build 7.2.0.1184</div>
</body></html>"""


class Handler(BaseHTTPRequestHandler):
    server_version = "IIS/6.0"

    def log_message(self, fmt, *args):
        pass

    def _log(self, status):
        rec = {
            "bot": getattr(self, "bot", ""),
            "ts": time.time(),
            "method": self.command,
            "path": self.path,
            "status": status,
            "ua": self.headers.get("User-Agent", ""),
            "ip": self.headers.get("Cf-Connecting-Ip") or self.client_address[0],
        }
        with LOCK, open(LOG_PATH, "a") as f:
            f.write(json.dumps(rec) + "\n")

    def _cookies(self):
        c = SimpleCookie()
        c.load(self.headers.get("Cookie", ""))
        return {k: v.value for k, v in c.items()}

    def _session(self, system):
        sid = self._cookies().get(f"{system}_sid")
        s = SESSIONS.get(sid) if sid else None
        return sid, s

    def _send(self, status, body, ctype="text/html; charset=utf-8", headers=None):
        if self.prefix and isinstance(body, str):
            body = LINK_RE.sub(lambda m: m.group(1) + self.prefix + "/" + m.group(2) + "/", body)
        if self.prefix and headers and headers.get("Location", "").startswith("/"):
            headers = {**headers, "Location": self.prefix + headers["Location"]}
        data = body.encode() if isinstance(body, str) else body
        self.send_response(status)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(data)))
        self.send_header("Cache-Control", "no-store")
        for k, v in (headers or {}).items():
            if isinstance(v, list):
                for item in v:
                    self.send_header(k, item)
            else:
                self.send_header(k, v)
        self.end_headers()
        self.wfile.write(data)
        self._log(status)

    def _redirect(self, loc, cookies=None):
        self._send(302, "", headers={"Location": loc, "Set-Cookie": cookies or []})

    def _form(self):
        n = int(self.headers.get("Content-Length", "0") or 0)
        raw = self.rfile.read(n).decode() if n else ""
        return {k: v[0] for k, v in parse_qs(raw, keep_blank_values=True).items()}

    def do_GET(self):
        self.route("GET")

    def do_POST(self):
        self.route("POST")

    def route(self, method):
        # /b/<bot-id>/... tags traffic per bot; links and redirects keep the prefix.
        self.prefix, self.bot = "", ""
        m = re.match(r"^/b/([A-Za-z0-9_-]+)(/.*)?$", self.path)
        if m:
            self.bot = m.group(1)
            self.prefix = "/b/" + self.bot
            self.path = m.group(2) or "/"
        u = urlparse(self.path)
        q = {k: v[0] for k, v in parse_qs(u.query).items()}
        p = u.path.rstrip("/") or "/"
        if p == "/":
            return self._send(200, page("Intranet", """<h3>Corporate Intranet</h3><ul>
<li><a href="/crm/">Acme CRM</a></li><li><a href="/billing/">BillPro Enterprise</a></li>
<li><a href="/helpdesk/">HelpDesk Classic</a></li></ul>""", "Intranet Portal"))
        if p.startswith("/crm"):
            return self.crm(method, p, q)
        if p.startswith("/billing"):
            return self.billing(method, p, q)
        if p.startswith("/helpdesk"):
            return self.helpdesk(method, p, q)
        self._send(404, page("Not found", "404 - File or directory not found.", "Error"))

    # ---------------- CRM ----------------
    def crm(self, method, p, q):
        title = "Acme CRM 7.2"
        nav = '<a href="/crm/customers">Customers</a><a href="/crm/logout">Log off</a>'
        if p in ("/crm", "/crm/login"):
            if method == "POST":
                f = self._form()
                csrf = self._cookies().get("crm_csrf")
                if not csrf or f.get("__RequestVerificationToken") != csrf:
                    return self._send(400, page(title, '<p class="err">Request verification failed. Reload the login page.</p>', title))
                if f.get("j_username") == CREDENTIALS["crm"]["username"] and f.get("j_password") == CREDENTIALS["crm"]["password"]:
                    sid = secrets.token_hex(12)
                    SESSIONS[sid] = {"system": "crm"}
                    return self._redirect("/crm/customers", [f"crm_sid={sid}; Path=/"])
                err = '<p class="err">Invalid user ID or password.</p>'
            else:
                err = ""
            token = secrets.token_hex(8)
            body = f"""{err}<form method="post" action="/crm/login">
<input type="hidden" name="__RequestVerificationToken" value="{token}">
<table><tr><td>User ID:</td><td><input name="j_username" id="uid"></td></tr>
<tr><td>Passcode:</td><td><input name="j_password" id="pwd" type="password"></td></tr>
<tr><td></td><td><input type="submit" value="Sign On"></td></tr></table></form>"""
            return self._send(200, page(title + " - Sign On", body, title),
                              headers={"Set-Cookie": f"crm_csrf={token}; Path=/"})
        _, s = self._session("crm")
        if not s:
            return self._redirect("/crm/login")
        if p == "/crm/logout":
            return self._redirect("/crm/login", ["crm_sid=; Path=/; Max-Age=0"])
        if p == "/crm/customers":
            per = 12
            qs = q.get("q", "").strip().lower()
            rows = [c for c in CUSTOMERS if qs in c["name"].lower()] if qs else CUSTOMERS
            pg = max(1, int(q.get("page", "1") or 1))
            pages = max(1, (len(rows) + per - 1) // per)
            chunk = rows[(pg - 1) * per: pg * per]
            trs = "".join(f"<tr><td>{c['id']}</td><td><a href='/crm/customer?id={c['id']}'>{html.escape(c['name'])}</a></td><td>{c['phone']}</td></tr>" for c in chunk)
            pager = " ".join(f"<a href='/crm/customers?page={i}&q={html.escape(qs)}'>[{i}]</a>" if i != pg else f"<b>{i}</b>" for i in range(1, pages + 1))
            body = f"""<form method="get" action="/crm/customers">Find account: <input name="q" value="{html.escape(qs)}"> <input type="submit" value="Go"></form>
<p>Page {pg} of {pages} ({len(rows)} records)</p>
<table class="grid"><tr><th>Acct #</th><th>Account Name</th><th>Main Phone</th></tr>{trs}</table><p>{pager}</p>"""
            return self._send(200, page(title + " - Accounts", body, title, nav))
        if p == "/crm/customer":
            c = CUST_BY_ID.get(int(q.get("id", "0") or 0))
            if not c:
                return self._send(404, page(title, "Record not found.", title, nav))
            body = f"""<h3>{html.escape(c['name'])}</h3><table class="grid">
<tr><th>Acct #</th><td>{c['id']}</td></tr>
<tr><th>Service Level</th><td>{c['tier']}</td></tr>
<tr><th>Relationship Owner</th><td>{c['manager']}</td></tr>
<tr><th>Contract End</th><td>{c['renewal']}</td></tr>
<tr><th>Licensed Seats</th><td>{c['seats']}</td></tr>
<tr><th>Billing Ref</th><td>{c['billing_account']}</td></tr>
<tr><th>Main Phone</th><td>{c['phone']}</td></tr></table>"""
            return self._send(200, page(title + " - " + c["name"], body, title, nav))
        self._send(404, page(title, "Not found", title, nav))

    # ---------------- Billing ----------------
    def billing(self, method, p, q):
        title = "BillPro Enterprise"
        nav = '<a href="/billing/accounts">Account Lookup</a><a href="/billing/logout">Exit</a>'
        creds = CREDENTIALS["billing"]
        sid, s = self._session("billing")
        if p in ("/billing", "/billing/login"):
            if method == "POST":
                f = self._form()
                if f.get("user") == creds["username"]:
                    sid = secrets.token_hex(12)
                    SESSIONS[sid] = {"system": "billing", "stage": "password"}
                    return self._redirect("/billing/verify", [f"billing_sid={sid}; Path=/"])
                return self._send(200, page(title, '<p class="err">Unknown operator.</p><a href="/billing/login">Back</a>', title))
            body = """<p>Step 1 of 2: identify yourself</p><form method="post" action="/billing/login">
Operator ID: <input name="user"> <button type="submit">Next &gt;</button></form>"""
            return self._send(200, page(title + " - Login", body, title))
        if not s:
            return self._redirect("/billing/login")
        if p == "/billing/verify":
            if method == "POST":
                f = self._form()
                if f.get("pw") == creds["password"] and f.get("sq", "").strip().lower() == creds["security_answer"].lower():
                    s["stage"] = "notice"
                    return self._redirect("/billing/notice")
                return self._send(200, page(title, '<p class="err">Verification failed.</p><a href="/billing/verify">Retry</a>', title))
            body = """<p>Step 2 of 2: verify</p><form method="post" action="/billing/verify">
<table><tr><td>Password</td><td><input type="password" name="pw"></td></tr>
<tr><td>Security question: <i>City of birth?</i></td><td><input name="sq"></td></tr></table>
<button type="submit">Log In</button></form>"""
            return self._send(200, page(title + " - Verify", body, title))
        if s.get("stage") == "password":
            return self._redirect("/billing/verify")
        if p == "/billing/notice":
            if method == "POST":
                s["stage"] = "in"
                return self._redirect("/billing/accounts")
            body = """<h3>System Notice</h3><p>Scheduled maintenance on Saturday 02:00-06:00 UTC.
Invoices posted during maintenance appear the next business day.</p>
<form method="post" action="/billing/notice"><label><input type="checkbox" required> I have read this notice</label>
<button type="submit">Continue</button></form>"""
            return self._send(200, page(title, body, title))
        if s.get("stage") != "in":
            return self._redirect("/billing/notice")
        if p == "/billing/logout":
            SESSIONS.pop(sid, None)
            return self._redirect("/billing/login", ["billing_sid=; Path=/; Max-Age=0"])
        if p == "/billing/accounts":
            acct = q.get("acct", "").strip().upper()
            if acct:
                if acct in INVOICES:
                    return self._redirect(f"/billing/account/{acct}")
                msg = f'<p class="err">No account {html.escape(acct)}.</p>'
            else:
                msg = ""
            body = f"""{msg}<form method="get" action="/billing/accounts">Billing account no. (BA-#####): <input name="acct">
<button type="submit">Lookup</button></form><p><i>Listing all accounts is disabled for performance reasons.</i></p>"""
            return self._send(200, page(title + " - Lookup", body, title, nav))
        if p.startswith("/billing/api/summary/"):
            acct = p.rsplit("/", 1)[-1]
            if acct not in INVOICES:
                return self._send(404, "{}", "application/json")
            time.sleep(1.5)
            return self._send(200, json.dumps({"outstanding": outstanding(acct), "currency": "USD"}), "application/json")
        if p.startswith("/billing/account/"):
            acct = p.rsplit("/", 1)[-1]
            if acct not in INVOICES:
                return self._send(404, page(title, "No such account", title, nav))
            c = CUST_BY_ACCT[acct]
            rows = "".join(f"<tr><td>{i['number']}</td><td>{i['issued']}</td><td>{i['due']}</td><td align=right>{i['amount']:,.2f}</td><td>{i['status']}</td></tr>" for i in INVOICES[acct])
            body = f"""<h3>Account {acct}</h3><p>Customer: {html.escape(c['name'])}</p>
<p>Outstanding balance: <b id="bal">calculating…</b></p>
<table class="grid"><tr><th>Invoice</th><th>Issued</th><th>Due</th><th>Amount (USD)</th><th>State</th></tr>{rows}</table>
<script>setTimeout(function(){{fetch('/billing/api/summary/{acct}').then(r=>r.json()).then(d=>{{
document.getElementById('bal').textContent='USD '+d.outstanding.toLocaleString('en-US',{{minimumFractionDigits:2}});}});}},500);</script>"""
            return self._send(200, page(title + " - " + acct, body, title, nav))
        self._send(404, page(title, "Not found", title, nav))

    # ---------------- Helpdesk ----------------
    def helpdesk(self, method, p, q):
        title = "HelpDesk Classic"
        nav = '<a href="/helpdesk/tickets">Tickets</a><a href="/helpdesk/logout">Logout</a>'
        creds = CREDENTIALS["helpdesk"]
        if p in ("/helpdesk", "/helpdesk/login"):
            err = ""
            if method == "POST":
                f = self._form()
                if f.get("domain") != creds["domain"]:
                    err = '<p class="err">Account not found in selected domain.</p>'
                elif f.get("login") == creds["username"] and f.get("secret") == creds["password"]:
                    sid = secrets.token_hex(12)
                    SESSIONS[sid] = {"system": "helpdesk"}
                    return self._redirect("/helpdesk/tickets", [f"helpdesk_sid={sid}; Path=/"])
                else:
                    err = '<p class="err">Login failed.</p>'
            body = f"""{err}<form method="post" action="/helpdesk/login"><table>
<tr><td>Login</td><td><input name="login"></td></tr>
<tr><td>Password</td><td><input type="password" name="secret"></td></tr>
<tr><td>Domain</td><td><select name="domain"><option>LOCAL</option><option>PARTNERS</option><option>CORP</option></select></td></tr>
</table><input type="submit" value="Login"></form>"""
            return self._send(200, page(title, body, title))
        _, s = self._session("helpdesk")
        if not s:
            return self._redirect("/helpdesk/login")
        if p == "/helpdesk/logout":
            return self._redirect("/helpdesk/login", ["helpdesk_sid=; Path=/; Max-Age=0"])
        if p == "/helpdesk/tickets":
            cust = q.get("customer", "").strip().lower()
            status = q.get("status", "")
            rows = TICKETS
            if cust:
                rows = [t for t in rows if cust in CUST_BY_ID[t["customer_id"]]["name"].lower()]
            if status:
                rows = [t for t in rows if t["status"] == status]
            rows = rows[:25]
            trs = "".join(f"<tr><td><a href='/helpdesk/ticket?id={t['id']}'>#{t['id']}</a></td><td>{html.escape(CUST_BY_ID[t['customer_id']]['name'])}</td><td>{t['subject']}</td><td>{t['status']}</td></tr>" for t in rows)
            body = f"""<form method="get" action="/helpdesk/tickets">Customer contains: <input name="customer" value="{html.escape(cust)}">
Status: <select name="status"><option value="">(any)</option><option>Open</option><option>Pending Customer</option><option>Closed</option></select>
<input type="submit" value="Filter"></form><p>Showing up to 25 tickets.</p>
<table class="grid"><tr><th>Ticket</th><th>Customer</th><th>Subject</th><th>Status</th></tr>{trs}</table>"""
            return self._send(200, page(title + " - Tickets", body, title, nav))
        if p == "/helpdesk/ticket":
            t = TICKET_BY_ID.get(int(q.get("id", "0") or 0))
            if not t:
                return self._send(404, page(title, "Ticket not found", title, nav))
            c = CUST_BY_ID[t["customer_id"]]
            body = f"""<h3>Ticket #{t['id']}: {html.escape(t['subject'])}</h3><table class="grid">
<tr><th>Customer</th><td>{html.escape(c['name'])} (CRM {c['id']})</td></tr>
<tr><th>Urgency</th><td>{t['priority']}</td></tr><tr><th>State</th><td>{t['status']}</td></tr>
<tr><th>Opened</th><td>{t['opened']}</td></tr></table>
<h4>Audit trail</h4><iframe src="/helpdesk/ticket/history?id={t['id']}" width="600" height="160"></iframe>"""
            return self._send(200, page(title + f" - #{t['id']}", body, title, nav))
        if p == "/helpdesk/ticket/history":
            t = TICKET_BY_ID.get(int(q.get("id", "0") or 0))
            if not t:
                return self._send(404, "not found")
            trs = "".join(f"<tr><td>{h['at']}</td><td>{h['status']}</td><td>{h['by']}</td></tr>" for h in t["history"])
            return self._send(200, f"<html><body style='font:11px Tahoma'><table border=1 cellspacing=0><tr><th>Date</th><th>Status</th><th>Changed by</th></tr>{trs}</table></body></html>")
        self._send(404, page(title, "Not found", title, nav))


def main():
    global LOG_PATH
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=8765)
    ap.add_argument("--log", default="access.log")
    ap.add_argument("--dump-data", help="write generated data to this JSON file and exit")
    a = ap.parse_args()
    if a.dump_data:
        with open(a.dump_data, "w") as f:
            json.dump({"customers": CUSTOMERS, "invoices": INVOICES, "tickets": TICKETS,
                       "outstanding": {k: outstanding(k) for k in INVOICES}}, f, indent=1)
        return
    LOG_PATH = a.log
    ThreadingHTTPServer(("0.0.0.0", a.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
