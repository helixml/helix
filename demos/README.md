## Tools demos

These are example APIs that the Helix can interact with using it's tools feature.

The APIs are intended to showcase how large language models can interact with enterprise data living behind inside on-prem services.

 * [Product Inventory](product-inventory)
 * [Hiring Interview](hiring-interview)
 * [Sales Pipeline](sales-pipeline)

### Product Inventory

Realtime inventory and booking system.

 * buy slenovo laptops, < 2k, next thursday
   * list of products
 * book it
 * fake invoice to email

Product schema:

```go
type Product struct {
  ID string `json:"id"`
  Name string `json:"name"`
  Description string `json:"description"`
  Price int `json:"price"`
  DeliveryLeadtime int `json:"delivery_leadtime"`
  InStock int `json:"in_stock"`
}
```

Receipt schema:

```go
type Receipt struct {
  ID string `json:"id"`
  ProductID string `json:"product_id"`
  CustomerEmail string `json:"customer_email"`
}
```

Endpoints:

 * `/products/v1/list` - replies with `[]Product`
   * `?min_price` (int)
   * `?max_price` (int)
   * `?delivery_before` (date)
 * `/products/v1/book`
   * `?product_id` (int)
   * `?customer_email` (string)

### Hiring Interview

There is a job vacancy and the HR system is keeping track of candidates.

We need to be able to:
 
 * show me all current vacancies and their status
 * for a vacancy, list the current candidates
   * each candidate will have a CV and Notes
 * for a candidate, send an email to prepare the candidate
   * we can personalise the email based on the job advert, their CV and notes
 * for a candidate, get some interview questions

### Sales Pipeline

The CRM system shows a deal that is far down the sales pipeline.

We want to know what the status is and get a good follow up email to send them.

 * what’s the status of the Citibank deal?
 * value of deal $1M
 * key contact details
 * draft me a followup email