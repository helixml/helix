package main

import (
	"encoding/json"
	"net/http"
)

type JobVacancyCandidate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	JobTitle       string `json:"jobTitle"`
	JobDescription string `json:"jobDescription"`
	CV             string `json:"description"`
}

type JobCandidateQuery struct {
	JobTitle string
}

var JOB_VACANCY_CANDIDATES = []JobVacancyCandidate{
	{
		ID:             "1",
		Name:           "John Doe",
		Email:          "john.doe@email.com",
		JobTitle:       "Software Engineer",
		JobDescription: "We are looking for a software engineer to join our team - they must be able to code in Go",
		CV:             "I am a software engineer with 5 years of experience and I can code in Go",
	},
}

func parseCandidateListParameters(params map[string][]string) JobCandidateQuery {
	var query JobCandidateQuery

	if val, ok := params["job_title"]; ok && len(val[0]) > 0 {
		query.JobTitle = val[0]
	}

	return query
}

func filterCandidates(candidates []JobVacancyCandidate, query JobCandidateQuery) []JobVacancyCandidate {
	var filtered []JobVacancyCandidate
	for _, candidate := range candidates {
		if len(query.JobTitle) > 0 && candidate.JobTitle != query.JobTitle {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func listCandidates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	query := parseCandidateListParameters(r.URL.Query())
	filteredCandidates := filterCandidates(JOB_VACANCY_CANDIDATES, query)
	json.NewEncoder(w).Encode(filteredCandidates)
}
