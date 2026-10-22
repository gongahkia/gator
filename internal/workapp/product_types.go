package workapp

type RecallStatus struct {
	Provider, State, Detail  string
	Storage, Retrieval, Chat string
	Ready                    bool
}

type KnowledgeItem struct {
	SourceReviewNeeded                                         bool
	ID, Kind, Title, Text, Scope, Source, Updated, CurrentText string
	Available                                                  bool
}

type ContextState struct {
	WorkID, GoalID, Outcome, Permission, Recall, Usage string
	Items                                              []KnowledgeItem
	Sources                                            []string
}
