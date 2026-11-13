package workapp

import "github.com/gongahkia/gator/internal/worksession"

// Review is generated from current account configuration. Change never crosses
// the frontend boundary; its token binds the displayed selection and readiness.
type RequestAccessChoice struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Kind        string `json:"kind"`
	Destination string `json:"destination"`
	Operation   string `json:"operation"`
	Permission  string `json:"permission"`
	Issue       string `json:"issue,omitempty"`
}
type RequestAccessRequirement struct {
	Need     worksession.TaskAccessNeed `json:"need"`
	Selected string                     `json:"selected,omitempty"`
	Choices  []RequestAccessChoice      `json:"choices"`
}
type RequestAccessReview struct {
	Token        string                        `json:"token"`
	Requirements []RequestAccessRequirement    `json:"requirements"`
	Connections  []string                      `json:"connections"`
	Ready        bool                          `json:"ready"`
	Summary      string                        `json:"summary"`
	Change       *worksession.PlanAccessChange `json:"-"`
}
