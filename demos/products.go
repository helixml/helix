package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/davecgh/go-spew/spew"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
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
	ReceiptID     string `json:"receipt_id"`
}

var ProductData = []Product{
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

func filterProducts(products []Product, query ProductQuery) []Product {
	var filtered []Product
	for _, product := range products {
		// THESE ARE ALL FILTERING OUT
		if query.MinPrice > 0 && product.Price < query.MinPrice {
			continue
		}
		if query.MaxPrice > 0 && product.Price > query.MaxPrice {
			continue
		}
		if !doesQueryMatchString(product.CPU, query.CPU) {
			continue
		}
		if query.RAM > 0 && product.RAM < query.RAM {
			continue
		}
		filtered = append(filtered, product)
	}
	return filtered
}

func listProducts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := r.URL.Query()
	query := ProductQuery{
		MinPrice: getQueryParamInteger("min_price", params),
		MaxPrice: getQueryParamInteger("max_price", params),
		CPU:      getQueryParamString("cpu", params, []string{"i5", "i7", "i9"}),
		RAM:      getQueryParamInteger("ram", params),
	}
	filteredProducts := filterProducts(ProductData, query)
	fmt.Printf("listProducts --------------------------------------\n")
	spew.Dump(query)
	spew.Dump(filteredProducts)
	if err := json.NewEncoder(w).Encode(filteredProducts); err != nil {
		log.Error().Msgf("failed streaming filtered products: %v", err)
	}
}

func bookProduct(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := r.URL.Query()
	query := PurchaseQuery{
		ProductID:     getQueryParamString("product_id", params, []string{"SX67", "SX88", "SX99"}),
		CustomerEmail: getQueryParamStringAny("customer_email", params),
	}
	fmt.Printf("purchaseProduct --------------------------------------\n")
	spew.Dump(query)
	if err := json.NewEncoder(w).Encode(Receipt{
		ProductID:     query.ProductID,
		CustomerEmail: query.CustomerEmail,
		ReceiptID:     "receipt-" + uuid.New().String(), // Always generate a new receipt ID
	}); err != nil {
		log.Error().Msgf("failed streaming purchased products: %v", err)
	}
}
