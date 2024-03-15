package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/davecgh/go-spew/spew"
)

// Product struct defines the structure of a product object
type Product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Price       int    `json:"price"`
	CPU         string `json:"cpu"`
	RAM         int    `json:"ram"`
	Inventory   int    `json:"inventory"`
}

type ProductQuery struct {
	MinPrice int
	MaxPrice int
	CPU      string
	RAM      int
}

type PurchaseQuery struct {
	ProductID     string
	CustomerEmail string
}

// Receipt struct defines the structure of a receipt object
type Receipt struct {
	ProductID     string `json:"product_id"`
	CustomerEmail string `json:"customer_email"`
}

var PRODUCT_DATA = []Product{
	{
		ID:          "1",
		Name:        "SX67",
		Description: "Medium performance laptop",
		Price:       1200,
		CPU:         "i5",
		RAM:         16,
		Inventory:   18,
	},
	{
		ID:          "1",
		Name:        "SX88",
		Description: "High performance laptop",
		Price:       1500,
		CPU:         "i7",
		RAM:         32,
		Inventory:   10,
	},
	{
		ID:          "2",
		Name:        "SX99",
		Description: "Ultra performance laptop",
		Price:       2500,
		CPU:         "i9",
		RAM:         64,
		Inventory:   2,
	},
}

func parseListParameters(params map[string][]string) ProductQuery {
	var query ProductQuery

	if val, ok := params["min_price"]; ok && len(val[0]) > 0 {
		minPrice, err := strconv.Atoi(val[0])
		if err != nil {
			minPrice = 0
		}
		query.MinPrice = minPrice
	}

	if val, ok := params["max_price"]; ok && len(val[0]) > 0 {
		maxPrice, err := strconv.Atoi(val[0])
		if err != nil {
			maxPrice = 0
		}
		query.MaxPrice = maxPrice
	}

	if val, ok := params["cpu"]; ok && len(val[0]) > 0 {
		query.CPU = val[0]
	}

	if val, ok := params["ram"]; ok && len(val[0]) > 0 {
		ram, err := strconv.Atoi(val[0])
		if err != nil {
			ram = 0
		}
		query.RAM = ram
	}

	return query
}

func filterProducts(products []Product, query ProductQuery) []Product {
	var filtered []Product
	for _, product := range products {
		if query.MinPrice > 0 && product.Price < query.MinPrice {
			continue
		}
		if query.MaxPrice > 0 && product.Price > query.MaxPrice {
			continue
		}
		if len(query.CPU) > 0 && product.CPU != query.CPU {
			continue
		}
		if query.RAM > 0 && product.RAM != query.RAM {
			continue
		}
		filtered = append(filtered, product)
	}
	return filtered
}

func listProducts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := parseListParameters(r.URL.Query())
	filteredProducts := filterProducts(PRODUCT_DATA, query)
	fmt.Printf("listProducts --------------------------------------\n")
	spew.Dump(query)
	spew.Dump(filteredProducts)
	json.NewEncoder(w).Encode(filteredProducts)
}

func parsePurchaseParameters(params map[string][]string) PurchaseQuery {
	var query PurchaseQuery

	if val, ok := params["product_id"]; ok && len(val[0]) > 0 {
		query.ProductID = val[0]
	}

	if val, ok := params["customer_email"]; ok && len(val[0]) > 0 {
		query.CustomerEmail = val[0]
	}

	return query
}

func bookProduct(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := parsePurchaseParameters(r.URL.Query())
	fmt.Printf("bookProduct --------------------------------------\n")
	spew.Dump(query)
	json.NewEncoder(w).Encode(PurchaseQuery{
		ProductID:     query.ProductID,
		CustomerEmail: query.CustomerEmail,
	})
}
