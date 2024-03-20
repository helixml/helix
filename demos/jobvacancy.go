package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/davecgh/go-spew/spew"
)

type JobVacancyCandidate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	JobTitle       string `json:"jobTitle"`
	JobDescription string `json:"jobDescription"`
	CV             string `json:"description"`
}

type JobCandidateQuery struct {
	JobTitle      string
	CandidateName string
}

var JOB_VACANCY_CANDIDATES = []JobVacancyCandidate{
	{
		ID:             "1",
		Name:           "Alice Bennett",
		JobTitle:       "Digital Marketing Specialist",
		JobDescription: "We are on the hunt for a highly-creative Digital Marketing Specialist to lead our marketing team. Responsibilities include developing digital campaigns to promote a product or service across all digital channels, analyzing user engagement data, optimizing marketing campaigns, and collaborating with the graphic design department to ensure consistent branding. Ideal candidates will have experience with social media management, email marketing, and content creation, along with a keen eye for analytics to track the effectiveness of marketing campaigns and identify areas for improvement.",
		CV:             "I am an enthusiastic and creative Digital Marketing Specialist with over 7 years of experience crafting compelling marketing campaigns that engage and inform users. Proficient in a wide range of digital marketing tools including Google Analytics, Hootsuite, MailChimp, and Adobe Creative Suite, I have a proven track record of growing online presence and brand awareness through well-executed marketing strategies. I hold a Bachelor’s degree in Marketing and have completed a professional course in Social Media Management. My previous roles have allowed me to develop strong skills in project management, team collaboration, and strategic planning.",
	},
	{
		ID:             "2",
		Name:           "Marcus Patel",
		JobTitle:       "Human Resources Manager",
		JobDescription: "We are looking for an experienced and professional Human Resources Manager to join our team. The HR Manager will lead all HR’s practices and objectives that will provide an employee-oriented, high-performance culture. This includes overseeing recruitment efforts, employee relations, regulatory compliance, and training and development. The ideal candidate should have a deep knowledge of HR principles, proven experience in conflict resolution and the ability to manage multiple priorities in a dynamic environment.",
		CV:             "As a dedicated Human Resources professional with over 10 years of experience, I have a strong foundation in all facets of HR management from recruitment to retirement. My expertise lies in developing effective HR strategies, enhancing workforce performance, streamlining HR processes, and fostering a positive work environment. I hold a Master's degree in Human Resources Management and am certified as a Professional in Human Resources (PHR). I have successfully implemented HR policies that have improved employee productivity, engagement, and satisfaction in my previous roles.",
	},
	{
		ID:             "3",
		Name:           "Sophia Li",
		JobTitle:       "Data Scientist",
		JobDescription: "We are seeking a skilled Data Scientist to join our team. This position involves leveraging data-driven techniques to solve business challenges, improve company’s products and services, and enhance decision-making processes. Responsibilities include applying data mining techniques, performing statistical analysis, and building high-quality prediction systems integrated with our products. The ideal candidate should have a strong background in machine learning, proficiency in programming languages like Python and R, and experience in using SQL for data extraction, manipulation, and retrieval.",
		CV:             "I am a seasoned Data Scientist with an extensive 8-year background in analyzing large datasets, developing automated data processing systems, and contributing to the development of cutting-edge analytics platforms. My expertise includes predictive modeling, data visualization, and deploying machine learning algorithms to solve real-world problems. I hold a PhD in Computer Science with a focus on data mining and machine learning, and I am highly skilled in Python, R, SQL, and big data technologies like Hadoop and Spark. My previous work has resulted in significant cost savings and revenue increases for my employers by turning data into actionable insights.",
	},
}

func filterCandidates(candidates []JobVacancyCandidate, query JobCandidateQuery) []JobVacancyCandidate {
	var filtered []JobVacancyCandidate
	for _, candidate := range candidates {
		if !doesQueryMatchString(candidate.JobTitle, query.JobTitle) {
			continue
		}
		if !doesQueryMatchString(candidate.Name, query.CandidateName) {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func listCandidates(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	params := r.URL.Query()
	query := JobCandidateQuery{
		JobTitle:      getQueryParamStringAny("job_title", params),
		CandidateName: getQueryParamStringAny("candidate_name", params),
	}
	filteredCandidates := filterCandidates(JOB_VACANCY_CANDIDATES, query)
	fmt.Printf("filteredCandidates --------------------------------------\n")
	spew.Dump(query)
	spew.Dump(filteredCandidates)
	json.NewEncoder(w).Encode(filteredCandidates)
}
