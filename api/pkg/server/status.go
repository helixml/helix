package server

import (
	"net/http"

	"github.com/bacalhau-project/lilysaas/api/pkg/store"
)

type statusResponse struct {
	User    string `json:"user"`
	Credits int    `json:"credits"`
}

func (apiServer *LilysaasAPIServer) status(res http.ResponseWriter, req *http.Request) (statusResponse, error) {
	balanceTransfers, err := apiServer.Store.GetBalanceTransfers(req.Context(), store.GetBalanceTransfersQuery{
		Owner:     getRequestUser(req),
		OwnerType: "user",
	})
	if err != nil {
		return statusResponse{}, err
	}

	// add up the total value of all balance transfers
	credits := 0
	for _, balanceTransfer := range balanceTransfers {
		credits += balanceTransfer.Amount
	}
	return statusResponse{
		User:    getRequestUser(req),
		Credits: credits,
	}, nil
}
