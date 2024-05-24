package store

import (
	"fmt"

	_ "github.com/lib/pq"

	"gorm.io/gorm"

	_ "github.com/doug-martin/goqu/v9/dialect/postgres"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
)

// the poing of this setup is if when we change database schemas we need to loop over
// existing data and update it somehow - i.e. when a schema migration also requires
// some code to run
// we have a table that keeps track of which "script" we have run
// in reality - each script is just a function that accepts a DB connection
// and can do what it wants

func helloWorld(*gorm.DB) error {
	fmt.Printf("HELLO --------------------------------------\n")
	return nil
}

var MIGRATION_SCRIPTS map[string]func(*gorm.DB) error = map[string]func(*gorm.DB) error{
	"01_hello_world": helloWorld,
}
