package server

import (
	"encoding/json"
	"net/http"

	"github.com/rs/zerolog/log"
)

func (apiServer *LilysaasAPIServer) status(res http.ResponseWriter, req *http.Request) {
	res.Header().Set("Access-Control-Allow-Origin", "*")

	err := func() error {
		job, err := apiServer.Controller.Store.GetJob(req.Context(), "")
		if err != nil {
			return err
		}

		err = json.NewEncoder(res).Encode(job)
		if err != nil {
			return err
		}

		return nil
	}()

	if err != nil {
		log.Ctx(req.Context()).Error().Msgf("error for register route: %s", err.Error())
		http.Error(res, err.Error(), http.StatusInternalServerError)
		return
	}
}
