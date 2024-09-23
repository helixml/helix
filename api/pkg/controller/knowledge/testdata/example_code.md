Webhook Relay detects multipart/formdata requests and automatically parses them so your function can use it. Parsed form data can be accessed through `r.RequestFormData` variable. You can use Webhook Relay to receive a form and convert it into any kind of JSON that can be sent to another API.

[**Using decoded values**](#Using-decoded-values)

For example if the payload fragment looks like this:

```
...
  --------------------------5683f7544dff7b07
  Content-Disposition: form-data; name="username"

  John
  --------------------------5683f7544dff7b07
  ...
```

Then to get `username` value (which is `John`) you will need to:

```
-- import "json" package when working with JSON
  local json = require("json")

  -- values can be accessed through 'r.RequestFormData' object. Since
  -- there can be multiple values for each key, you also need to
  -- specify that it's the first element of the list:
  local username = r.RequestFormData.username[1]
  local first_name = r.RequestFormData.first_name[1]

  -- transforming form data into JSON
  local json_payload = {
    username = username,
    first_name = first_name
  }

  local encoded_payload, err = json.encode(json_payload)
  if err then error(err) end

  r:SetRequestBody(encoded_payload)
```

[**Prerequisites**](#Prerequisites)

For the decoding to work, Webhook Relay expects a header `Content-Type` that includes `multipart/form-data` and the boundary.