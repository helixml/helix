package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Port string `envconfig:"PORT" default:"8080"`
}

func main() {
	var config Config
	err := envconfig.Process("", &config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	r := mux.NewRouter()

	r.HandleFunc("/products/v1/list", listProducts).Methods("GET")
	r.HandleFunc("/products/v1/book", bookProduct).Methods("POST")

	r.HandleFunc("/jobvacancies/v1/list", listCandidates).Methods("GET")

	r.HandleFunc("/salesleads/v1/list", listSalesLeads).Methods("GET")

	log.Info().Str("port", config.Port).Msg("Starting server...")
	if err := http.ListenAndServe(fmt.Sprintf(":%s", config.Port), r); err != nil {
		log.Fatal().Err(err).Msg("Failed listening")
	}
}
