package system

import (
	"math/rand"
	"strconv"
)

var adjectives = []string{
	"enchanting",
	"fascinating",
	"elucidating",
	"useful",
	"helpful",
	"constructive",
	"charming",
	"playful",
	"whimsical",
	"delightful",
	"fantastical",
	"magical",
	"spellbinding",
	"dazzling",
}

var nouns = []string{
	"discussion",
	"dialogue",
	"convo",
	"conversation",
	"chat",
	"talk",
	"exchange",
	"debate",
	"conference",
	"seminar",
	"symposium",
}

func GenerateAmusingName() string {
	adj := adjectives[rand.Intn(len(adjectives))]
	noun := nouns[rand.Intn(len(nouns))]
	number := rand.Intn(900) + 100 // generates a random 3 digit number
	return adj + "-" + noun + "-" + strconv.Itoa(number)
}
