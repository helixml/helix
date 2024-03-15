package main

import (
	"encoding/json"
	"net/http"
)

type SalesLead struct {
	ID                 string `json:"id"`
	CompanyName        string `json:"company_name"`
	CompanyDescription string `json:"jobDescription"`
	ContactName        string `json:"contact_name"`
	JobTitle           string `json:"jobTitle"`
	Status             string `json:"status"`
	Notes              string `json:"notes"`
}

type SalesLeadQuery struct {
	Status string
}

var SALES_LEADS = []SalesLead{
	{
		ID:                 "1",
		CompanyName:        "ACME",
		CompanyDescription: "Sell things to Coyotes",
		ContactName:        "Coyote Snr",
		JobTitle:           "Procurement Officer",
		Status:             "active",
		Notes:              "Said they like ice cream",
	},
}

func parseSalesLeadListParameters(params map[string][]string) SalesLeadQuery {
	var query SalesLeadQuery

	if val, ok := params["status"]; ok && len(val[0]) > 0 {
		query.Status = val[0]
	}

	return query
}

func filterSalesLeads(salesLeads []SalesLead, query SalesLeadQuery) []SalesLead {
	var filtered []SalesLead
	for _, salesLead := range salesLeads {
		if len(query.Status) > 0 && salesLead.Status != query.Status {
			continue
		}
		filtered = append(filtered, salesLead)
	}
	return filtered
}

func listSalesLeads(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := parseSalesLeadListParameters(r.URL.Query())
	filteredSalesLeads := filterSalesLeads(SALES_LEADS, query)
	json.NewEncoder(w).Encode(filteredSalesLeads)
}
