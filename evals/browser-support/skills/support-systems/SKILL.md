---
name: support-systems
description: Site map, login steps, URL patterns and field-name glossary for the company's internal systems (Acme CRM, BillPro Enterprise billing, HelpDesk Classic tickets, Swag Labs store). Use whenever a support question mentions a customer, account, invoice, balance, ticket or product.
---

# Support systems playbook

`{{BASE}}` is the intranet base URL from your instructions. Credentials are in your instructions.

## Which system has what

| Question is about | System | Key |
|---|---|---|
| Account manager, tier / service level, contract end / renewal, seats, phone, billing account number | Acme CRM | customer name or CRM account # |
| Balance owed, invoices, overdue / open / paid | BillPro | billing account `BA-#####` |
| Tickets, priority, ticket status, who changed it | HelpDesk | ticket `#5xxx` or customer name |
| Products and prices | Swag Labs store | product name |

Cross-system questions: a **customer name** leads to the CRM, whose record shows the **Billing Ref** (`BA-#####`) for BillPro. A **ticket** shows the customer name and CRM account # (`CRM 10xx`), which opens the CRM record directly.

## Acme CRM — `{{BASE}}/crm/`

- Login page `{{BASE}}/crm/login`: fields **User ID**, **Passcode**, button **Sign On**. The form has a hidden token; always submit via the page, never by crafting a request.
- Search: `navigate_page` to `{{BASE}}/crm/customers?q=<one distinctive word of the name>`. Results link to the record.
- Record: `{{BASE}}/crm/customer?id=<CRM account #>`.
- The customer list (`{{BASE}}/crm/customers?page=N`, 12 per page, 6 pages) shows only Acct #, name and phone. Tier, owner, contract end and billing ref are **only on the record page** — for questions across many customers use the "Many records at once" technique from `browser-lookup`.
- CRM account numbers run from 1001 upwards in list order.
- Field glossary (the UI uses its own terms):
  - account manager / owner → **Relationship Owner**
  - tier / plan / level → **Service Level**
  - contract end / renewal date → **Contract End**
  - billing account → **Billing Ref**

## BillPro Enterprise — `{{BASE}}/billing/`

Login is three screens, in this order:
1. `{{BASE}}/billing/login` — **Operator ID**, click **Next >**.
2. Verify page — **Password** and **Security question** (city of birth), click **Log In**.
3. System Notice — tick **I have read this notice**, click **Continue**.

Then:
- Account page: `navigate_page` to `{{BASE}}/billing/account/<BA-#####>` directly. There is no account list.
- **Outstanding balance** loads a moment after the page: it first shows `calculating…`. Call `wait_for` with text `USD`, then snapshot. Report that figure; do not add up invoices yourself.
- The invoice table's **State** column is `Paid`, `Open` or `Overdue`. For "overdue" questions count only `Overdue` rows and sum their **Amount (USD)**.

## HelpDesk Classic — `{{BASE}}/helpdesk/`

- Login `{{BASE}}/helpdesk/login`: **Login**, **Password**, and the **Domain** dropdown must be set to `CORP` (the default `LOCAL` fails).
- Ticket: `{{BASE}}/helpdesk/ticket?id=<ticket number without #>`. **Urgency** = priority, **State** = current status. The **Customer** row shows the CRM account #.
- Status history ("who closed it", "status before X") is in an iframe. Open `{{BASE}}/helpdesk/ticket/history?id=<ticket number>` directly: rows are Date, Status and Changed by, oldest first.
- Tickets for a customer: `{{BASE}}/helpdesk/tickets?customer=<word from name>`.

## Swag Labs store — `https://www.saucedemo.com`

- Login fields **Username** and **Password**, button **Login**. The inventory page lists every product with its price.
- Cheapest or most expensive: read all prices from the one inventory snapshot. Sorting is not needed.
