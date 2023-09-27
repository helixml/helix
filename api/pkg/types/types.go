package types

import "time"

type Job struct {
	ID      string    `json:"id"`
	Created time.Time `json:"created"`
	State   string    `json:"state"`
	Status  string    `json:"status"`
	Data    string    `json:"data"`
}
