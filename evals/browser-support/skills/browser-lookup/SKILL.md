---
name: browser-lookup
description: Fast, reliable technique for answering a question by logging into a web application with the chrome-devtools browser tools and reading a value off the page. Use for every support lookup in CRM, billing, ticketing or any other internal web system.
---

# Browser lookup

You answer questions by reading internal web systems through the `chrome-devtools` MCP tools. Each tool call costs time and tokens: aim for the fewest calls that give a verified answer.

## Procedure

1. **Go straight to the record.** If the system playbook (`support-systems` skill) gives a URL pattern, `navigate_page` to it directly. Do not click through menus you do not need.
2. **Read with `take_snapshot`, not screenshots.** The snapshot is the page's text tree and has every value you need. Only use `take_screenshot` if the snapshot is missing content (for example, canvas charts).
3. **If you land on a login page, log in with one `fill_form` call** that fills every field at once, including dropdowns (pass the option text, e.g. `CORP`). Then click the submit button. Check the next snapshot for an error message before continuing.
4. **Values that load late:** if a field shows a placeholder such as `calculating…` or `Loading`, call `wait_for` with text that will appear (for example `USD`), then take one more snapshot.
5. **iframes:** snapshot content inside an iframe may be missing. Navigate to the iframe's `src` URL directly and snapshot that page.
6. **Stop as soon as you have the value.** Do not re-verify by visiting the same page again.

## Many records at once

When a question needs a field from many records (every customer of a manager, every account in a list), do not open the records one by one. Log in through the UI first, then make **one** `evaluate_script` call that fetches the record pages from the logged-in tab and extracts only the fields you need:

```js
async () => {
  const ids = [1001, 1002 /* … from the list page */];
  const out = [];
  for (const id of ids) {
    const html = await (await fetch(`{{BASE}}/crm/customer?id=${id}`)).text();
    const doc = new DOMParser().parseFromString(html, "text/html");
    const cell = (label) => [...doc.querySelectorAll("tr")]
      .find((tr) => tr.querySelector("th")?.textContent.trim() === label)
      ?.querySelector("td")?.textContent.trim();
    out.push({ id, name: doc.querySelector("h3")?.textContent, owner: cell("Relationship Owner"), level: cell("Service Level") });
  }
  return out;
}
```

`fetch` from the page reuses the browser's login session. Keep the returned JSON small: only the fields the question needs. Get the list of ids from the list or search pages first (they are paginated; read every page).

## Names that do not match

If no record matches the name exactly, say that no such customer/account was found, and mention close matches you saw. Never answer with a different record's data.

## Rules

- Use the browser tools, not `curl`, `wget` or scripts, to read the web systems.
- Never guess or compute a value the system displays. If the page shows a total, report that total.
- If a search returns nothing, try a shorter query (one distinctive word of the name) before giving up.
- If a session expired and you are redirected to login, log in again and retry the same URL.

## Answer format

Reply in one or two sentences with the exact values as displayed (names, dates, amounts with currency, statuses), and name the system each value came from. No preamble.
