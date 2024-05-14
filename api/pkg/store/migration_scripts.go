package store

import (
	"fmt"

	_ "github.com/lib/pq"

	"gorm.io/gorm"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
)

func helloWorld(*gorm.DB) error {
	fmt.Printf("HELLO --------------------------------------\n")
	return nil
}

var MIGRATION_SCRIPTS map[string]func(*gorm.DB) error = map[string]func(*gorm.DB) error{
	"01_hello_world": helloWorld,
}
