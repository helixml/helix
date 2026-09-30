# Connect invitation routing and browser flow

An intake has a server-generated `sci_` ID and a separate random invitation token. The create API and MCP tool return the ID with the URL. The URL path now carries the ID; its fragment carries the token. The browser sends both to the redeem endpoint automatically, and the store updates only the row matching both values. The token stays out of HTTP request URLs and referrer headers.

The browser then receives a short-lived HttpOnly flow cookie. The form route checks that the cookie's intake matches the ID in the path. Submission uses that row's project, customer, and conversation metadata. A second link opened in the same browser replaces the flow cookie, so only one intake form is active there at a time.

The ID is a correlation handle, not proof of customer identity. The current create API accepts `customer_id` and `conversation_id` from an authenticated project caller; it does not verify the person opening the link. A customer-facing gateway must derive both from its authenticated session and deliver the link to that customer. The manual DXB smoke test used placeholder IDs and dummy values, so it proves routing and form behavior, not end-customer authentication or website login.

Live verification on 2026-09-28: a new invitation opened the form without a Continue button; the URL retained the `sci_` ID after the fragment was removed; a dummy submission changed the matching intake status to `submitted`; revocation returned 204.
