package azure

import "time"

type Event struct {
	ID        string `json:"id"`
	EventType string `json:"eventType"`
}

type PullRequest struct {
	SubscriptionID string `json:"subscriptionId"`
	NotificationID int    `json:"notificationId"`
	ID             string `json:"id"`
	EventType      string `json:"eventType"`
	PublisherID    string `json:"publisherId"`
	Message        struct {
		Text     string `json:"text"`
		HTML     string `json:"html"`
		Markdown string `json:"markdown"`
	} `json:"message"`
	DetailedMessage struct {
		Text     string `json:"text"`
		HTML     string `json:"html"`
		Markdown string `json:"markdown"`
	} `json:"detailedMessage"`
	Resource struct {
		Repository struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			URL     string `json:"url"`
			Project struct {
				ID             string    `json:"id"`
				Name           string    `json:"name"`
				URL            string    `json:"url"`
				State          string    `json:"state"`
				Revision       int       `json:"revision"`
				Visibility     string    `json:"visibility"`
				LastUpdateTime time.Time `json:"lastUpdateTime"`
			} `json:"project"`
			Size            int    `json:"size"`
			RemoteURL       string `json:"remoteUrl"`
			SSHURL          string `json:"sshUrl"`
			WebURL          string `json:"webUrl"`
			IsDisabled      bool   `json:"isDisabled"`
			IsInMaintenance bool   `json:"isInMaintenance"`
		} `json:"repository"`
		PullRequestID int    `json:"pullRequestId"`
		CodeReviewID  int    `json:"codeReviewId"`
		Status        string `json:"status"`
		CreatedBy     struct {
			DisplayName string `json:"displayName"`
			URL         string `json:"url"`
			Links       struct {
				Avatar struct {
					Href string `json:"href"`
				} `json:"avatar"`
			} `json:"_links"`
			ID         string `json:"id"`
			UniqueName string `json:"uniqueName"`
			ImageURL   string `json:"imageUrl"`
			Descriptor string `json:"descriptor"`
		} `json:"createdBy"`
		CreationDate          time.Time `json:"creationDate"`
		Title                 string    `json:"title"`
		Description           string    `json:"description"`
		SourceRefName         string    `json:"sourceRefName"`
		TargetRefName         string    `json:"targetRefName"`
		MergeStatus           string    `json:"mergeStatus"`
		IsDraft               bool      `json:"isDraft"`
		MergeID               string    `json:"mergeId"`
		LastMergeSourceCommit struct {
			CommitID string `json:"commitId"`
			URL      string `json:"url"`
		} `json:"lastMergeSourceCommit"`
		LastMergeTargetCommit struct {
			CommitID string `json:"commitId"`
			URL      string `json:"url"`
		} `json:"lastMergeTargetCommit"`
		LastMergeCommit struct {
			CommitID string `json:"commitId"`
			Author   struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"author"`
			Committer struct {
				Name  string    `json:"name"`
				Email string    `json:"email"`
				Date  time.Time `json:"date"`
			} `json:"committer"`
			Comment string `json:"comment"`
			URL     string `json:"url"`
		} `json:"lastMergeCommit"`
		Reviewers []any  `json:"reviewers"`
		URL       string `json:"url"`
		Links     struct {
			Web struct {
				Href string `json:"href"`
			} `json:"web"`
			Statuses struct {
				Href string `json:"href"`
			} `json:"statuses"`
		} `json:"_links"`
		SupportsIterations bool   `json:"supportsIterations"`
		ArtifactID         string `json:"artifactId"`
	} `json:"resource"`
	ResourceVersion    string `json:"resourceVersion"`
	ResourceContainers struct {
		Collection struct {
			ID      string `json:"id"`
			BaseURL string `json:"baseUrl"`
		} `json:"collection"`
		Account struct {
			ID      string `json:"id"`
			BaseURL string `json:"baseUrl"`
		} `json:"account"`
		Project struct {
			ID      string `json:"id"`
			BaseURL string `json:"baseUrl"`
		} `json:"project"`
	} `json:"resourceContainers"`
	CreatedDate time.Time `json:"createdDate"`
}

type PullRequestComment struct {
	SubscriptionID string `json:"subscriptionId"`
	NotificationID int    `json:"notificationId"`
	ID             string `json:"id"`
	EventType      string `json:"eventType"`
	PublisherID    string `json:"publisherId"`
	Message        struct {
		Text     string `json:"text"`
		HTML     string `json:"html"`
		Markdown string `json:"markdown"`
	} `json:"message"`
	DetailedMessage struct {
		Text     string `json:"text"`
		HTML     string `json:"html"`
		Markdown string `json:"markdown"`
	} `json:"detailedMessage"`
	Resource struct {
		Comment struct {
			ID              int `json:"id"`
			ParentCommentID int `json:"parentCommentId"`
			Author          struct {
				DisplayName string `json:"displayName"`
				URL         string `json:"url"`
				Links       struct {
					Avatar struct {
						Href string `json:"href"`
					} `json:"avatar"`
				} `json:"_links"`
				ID         string `json:"id"`
				UniqueName string `json:"uniqueName"`
				ImageURL   string `json:"imageUrl"`
				Descriptor string `json:"descriptor"`
			} `json:"author"`
			Content                string    `json:"content"`
			PublishedDate          time.Time `json:"publishedDate"`
			LastUpdatedDate        time.Time `json:"lastUpdatedDate"`
			LastContentUpdatedDate time.Time `json:"lastContentUpdatedDate"`
			IsDeleted              bool      `json:"isDeleted"`
			CommentType            string    `json:"commentType"`
			UsersLiked             []any     `json:"usersLiked"`
			Links                  struct {
				Self struct {
					Href string `json:"href"`
				} `json:"self"`
				Repository struct {
					Href string `json:"href"`
				} `json:"repository"`
				Threads struct {
					Href string `json:"href"`
				} `json:"threads"`
				PullRequests struct {
					Href string `json:"href"`
				} `json:"pullRequests"`
			} `json:"_links"`
		} `json:"comment"`
		PullRequest struct {
			Repository struct {
				ID      string `json:"id"`
				Name    string `json:"name"`
				URL     string `json:"url"`
				Project struct {
					ID             string    `json:"id"`
					Name           string    `json:"name"`
					URL            string    `json:"url"`
					State          string    `json:"state"`
					Revision       int       `json:"revision"`
					Visibility     string    `json:"visibility"`
					LastUpdateTime time.Time `json:"lastUpdateTime"`
				} `json:"project"`
				Size            int    `json:"size"`
				RemoteURL       string `json:"remoteUrl"`
				SSHURL          string `json:"sshUrl"`
				WebURL          string `json:"webUrl"`
				IsDisabled      bool   `json:"isDisabled"`
				IsInMaintenance bool   `json:"isInMaintenance"`
			} `json:"repository"`
			PullRequestID int    `json:"pullRequestId"`
			CodeReviewID  int    `json:"codeReviewId"`
			Status        string `json:"status"`
			CreatedBy     struct {
				DisplayName string `json:"displayName"`
				URL         string `json:"url"`
				Links       struct {
					Avatar struct {
						Href string `json:"href"`
					} `json:"avatar"`
				} `json:"_links"`
				ID         string `json:"id"`
				UniqueName string `json:"uniqueName"`
				ImageURL   string `json:"imageUrl"`
				Descriptor string `json:"descriptor"`
			} `json:"createdBy"`
			CreationDate          time.Time `json:"creationDate"`
			Title                 string    `json:"title"`
			Description           string    `json:"description"`
			SourceRefName         string    `json:"sourceRefName"`
			TargetRefName         string    `json:"targetRefName"`
			MergeStatus           string    `json:"mergeStatus"`
			IsDraft               bool      `json:"isDraft"`
			MergeID               string    `json:"mergeId"`
			LastMergeSourceCommit struct {
				CommitID string `json:"commitId"`
				URL      string `json:"url"`
			} `json:"lastMergeSourceCommit"`
			LastMergeTargetCommit struct {
				CommitID string `json:"commitId"`
				URL      string `json:"url"`
			} `json:"lastMergeTargetCommit"`
			LastMergeCommit struct {
				CommitID string `json:"commitId"`
				Author   struct {
					Name  string    `json:"name"`
					Email string    `json:"email"`
					Date  time.Time `json:"date"`
				} `json:"author"`
				Committer struct {
					Name  string    `json:"name"`
					Email string    `json:"email"`
					Date  time.Time `json:"date"`
				} `json:"committer"`
				Comment string `json:"comment"`
				URL     string `json:"url"`
			} `json:"lastMergeCommit"`
			Reviewers          []any  `json:"reviewers"`
			URL                string `json:"url"`
			SupportsIterations bool   `json:"supportsIterations"`
			ArtifactID         string `json:"artifactId"`
		} `json:"pullRequest"`
	} `json:"resource"`
	ResourceVersion    string `json:"resourceVersion"`
	ResourceContainers struct {
		Collection struct {
			ID      string `json:"id"`
			BaseURL string `json:"baseUrl"`
		} `json:"collection"`
		Account struct {
			ID      string `json:"id"`
			BaseURL string `json:"baseUrl"`
		} `json:"account"`
		Project struct {
			ID      string `json:"id"`
			BaseURL string `json:"baseUrl"`
		} `json:"project"`
	} `json:"resourceContainers"`
	CreatedDate time.Time `json:"createdDate"`
}
