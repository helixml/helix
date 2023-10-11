package types

import "time"

type JobData struct {
	Module string            `json:"module"`
	Inputs map[string]string `json:"inputs"`
}

type Job struct {
	ID        string    `json:"id"`
	Created   time.Time `json:"created"`
	Owner     string    `json:"owner"`
	OwnerType string    `json:"owner_type"`
	State     string    `json:"state"`
	Status    string    `json:"status"`
	Data      JobData   `json:"data"`
}

type BalanceTransferData struct {
	StripePaymentID string `json:"stripe_payment_id"`
}

type BalanceTransfer struct {
	ID          string              `json:"id"`
	Created     time.Time           `json:"created"`
	Owner       string              `json:"owner"`
	OwnerType   string              `json:"owner_type"`
	PaymentType string              `json:"state"`
	Amount      int                 `json:"amount,string,omitempty"`
	Data        BalanceTransferData `json:"data"`
}

type Module struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Cost     int    `json:"cost"`
	Template string `json:"template"`
}
