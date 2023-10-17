package types

import (
	"context"
	"time"

	"github.com/bacalhau-project/lilypad/pkg/data"
)

type OwnerType string

const (
	OwnerTypeUser OwnerType = "user"
)

type PaymentType string

const (
	PaymentTypeAdmin  PaymentType = "admin"
	PaymentTypeStripe PaymentType = "stripe"
	PaymentTypeJob    PaymentType = "job"
)

type JobSpec struct {
	Module string            `json:"module"`
	Inputs map[string]string `json:"inputs"`
}

type JobData struct {
	Spec      JobSpec                `json:"spec"`
	Container data.JobOfferContainer `json:"container"`
}

type Job struct {
	ID        string    `json:"id"`
	Created   time.Time `json:"created"`
	Owner     string    `json:"owner"`
	OwnerType OwnerType `json:"owner_type"`
	State     string    `json:"state"`
	Status    string    `json:"status"`
	Data      JobData   `json:"data"`
}

type BalanceTransferData struct {
	JobID           string `json:"job_id"`
	StripePaymentID string `json:"stripe_payment_id"`
}

type BalanceTransfer struct {
	ID          string              `json:"id"`
	Created     time.Time           `json:"created"`
	Owner       string              `json:"owner"`
	OwnerType   OwnerType           `json:"owner_type"`
	PaymentType PaymentType         `json:"payment_type"`
	Amount      int                 `json:"amount"`
	Data        BalanceTransferData `json:"data"`
}

type Module struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Cost     int    `json:"cost"`
	Template string `json:"template"`
}

type Session struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Created        time.Time `json:"created"`
	Updated        time.Time `json:"updated"`
	Mode           string    `json:"mode"`
	Type           string    `json:"type"`
	FinetuneFile   string    `json:"finetune_file"`
	Interactions   string    `json:"interactions"`
	Owner          string    `json:"owner"`
	OwnerType      OwnerType `json:"owner_type"`
}

// passed between the api server and the controller
type RequestContext struct {
	Ctx       context.Context
	Owner     string
	OwnerType OwnerType
}

type UserStatus struct {
	User    string `json:"user"`
	Credits int    `json:"credits"`
}

type WebsocketEventType string

const (
	WebsocketEventJobUpdate WebsocketEventType = "job"
)

type WebsocketEvent struct {
	Type WebsocketEventType `json:"type"`
	Job  *Job               `json:"job"`
}
