package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/davecgh/go-spew/spew"
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
		CompanyName:        "Tech Innovations Inc.",
		CompanyDescription: "Tech Innovations Inc. specializes in developing cutting-edge software solutions for the finance sector, focusing on AI-driven risk management tools. They have a strong presence in over 20 countries and are recognized for their innovative approach to financial technology.",
		ContactName:        "Emma Clark",
		JobTitle:           "Chief Technology Officer",
		Status:             "active",
		Notes:              "Initial contact made at FinTech Innovations Conference 2023. Expressed interest in our data analytics services for enhancing their risk assessment models. Followed up with a demo presentation last month. Emma mentioned a potential partnership to integrate our technology with their existing products. Scheduled next meeting to discuss technical requirements and contract terms.",
	},
	{
		ID:                 "2",
		CompanyName:        "GreenWorld Dynamics",
		CompanyDescription: "An environmental consultancy firm providing sustainable solutions to reduce carbon footprint for businesses. They have been instrumental in implementing eco-friendly practices in over 100 companies globally.",
		ContactName:        "David Wei",
		JobTitle:           "Sustainability Officer",
		Status:             "active",
		Notes:              "Contacted via LinkedIn after noticing our work with similar eco-conscious companies. Discussed their current projects and how our products could align with their sustainability goals. David requested detailed case studies and a cost-benefit analysis report. Need to prepare customized proposal highlighting our successful projects in the renewable energy sector.",
	},
	{
		ID:                 "3",
		CompanyName:        "Gourmet Express",
		CompanyDescription: "Gourmet Express is a fast-growing chain of boutique fast-food outlets offering healthy, gourmet-quality meals at competitive prices. Currently operating in 30 locations nationwide and planning to expand internationally.",
		ContactName:        "Samantha Lee",
		JobTitle:           "Director of Operations",
		Status:             "active",
		Notes:              "Met Samantha at the National Restaurant Owners Conference. Expressed interest in our supply chain optimization services. Provided initial consultation and shared testimonials from other food industry clients. Samantha raised concerns about implementation timeline and training for staff. Agreed to arrange a trial at two locations with full support. Need to follow up to finalize the details and set a start date.",
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
	fmt.Printf("filteredSalesLeads --------------------------------------\n")
	spew.Dump(query)
	spew.Dump(filteredSalesLeads)
	json.NewEncoder(w).Encode(filteredSalesLeads)
}
