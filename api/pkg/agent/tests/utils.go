package tests

import gonanoid "github.com/matoous/go-nanoid/v2"

func GenerateNewTestID() string {
	newID, err := gonanoid.New()
	if err != nil {
		panic(err)
	}
	return "test" + newID
}
