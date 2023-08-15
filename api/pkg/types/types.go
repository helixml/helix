package types

type User struct {
	Email string `json:"email"`
}

type Module struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Images      []string `json:"images"`
	Price       int      `json:"price"` // in cents
}

type Job struct {
	ID string `json:"id"`
}
