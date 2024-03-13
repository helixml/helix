package main

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// Product struct defines the structure of a product object
type Product struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	Price            int    `json:"price"`
	DeliveryLeadtime int    `json:"delivery_leadtime"`
	InStock          int    `json:"in_stock"`
}

// Receipt struct defines the structure of a receipt object
type Receipt struct {
	ID            string `json:"id"`
	ProductID     string `json:"product_id"`
	CustomerEmail string `json:"customer_email"`
}

// Fixture data
var products = []Product{
	{
		ID:               "1",
		Name:             "Slenovo Laptop",
		Description:      "High performance laptop",
		Price:            1500,
		DeliveryLeadtime: 7,
		InStock:          10,
	},
	// Add more products as needed
}

func main() {
	r := mux.NewRouter()

	r.HandleFunc("/products/v1/list", listProducts).Methods("GET")
	r.HandleFunc("/products/v1/book", bookProduct).Methods("POST")

	http.ListenAndServe(":8080", r)
}

func listProducts(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := r.URL.Query()

	minPrice, maxPrice, deliveryBefore := parseQueryParameters(query)

	filteredProducts := filterProducts(minPrice, maxPrice, deliveryBefore)
	json.NewEncoder(w).Encode(filteredProducts)
}

func bookProduct(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var receipt Receipt
	err := json.NewDecoder(r.Body).Decode(&receipt)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Simulate booking logic and creation of a fake invoice here
	receipt.ID = "fake-receipt-id" // Generate a real ID for production use
	json.NewEncoder(w).Encode(receipt)
}

func parseQueryParameters(query map[string][]string) (int, int, time.Time) {
	// Add logic to parse and convert query parameters (min_price, max_price, delivery_before)
	// For simplicity, default values are set here. Add real parsing logic for production use.
	return 0, 2000, time.Now().AddDate(0, 0, 7) // Replace with real parsing logic
}

func filterProducts(minPrice, maxPrice int, deliveryBefore time.Time) []Product {
	// Add logic to filter products based on query parameters
	// This is simplified; add comprehensive filtering for production use.
	var filtered []Product
	for _, product := range products {
		if product.Price >= minPrice && product.Price <= maxPrice {
			filtered = append(filtered, product)
		}
	}
	return filtered
}
