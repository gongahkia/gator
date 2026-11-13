package workrun

import (
	"errors"
	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/worksession"
)

func classifyRequestRecovery(err error, readOnly bool) *worksession.RequestRecovery {
	if err == nil {
		return nil
	}
	if errors.Is(err, agent.ErrBudget) {
		return &worksession.RequestRecovery{Kind: "work_budget", Summary: "This request exhausted its execution budget.", Repair: "Inspect retained work and remaining limits before reviewing any additional work."}
	}
	var status agent.ProviderHTTPError
	if !errors.As(err, &status) {
		return nil
	}
	result := &worksession.RequestRecovery{Status: status.Status, RetrySafe: readOnly}
	switch status.Status {
	case 401:
		result.Kind = "credentials"
		result.Summary = "The selected model provider rejected this request's credentials."
		result.Repair = "Repair credentials for the original provider in Models, then prepare an explicit reviewed retry."
	case 403, 404:
		result.Kind = "model_access"
		result.Summary = "The provider rejected access to the selected model or endpoint."
		result.Repair = "Check model access and endpoint configuration in Models. Keep the original task scope when reviewing a retry."
	case 402:
		result.Kind = "provider_budget"
		result.Summary = "The model provider rejected the request for an account or credit requirement."
		result.Repair = "Inspect the provider account's limits, then return to review the original request."
	case 429:
		result.Kind = "rate_limit"
		result.Summary = "The selected provider is still rate limiting this request after bounded retries."
		result.Repair = "Wait for provider capacity or repair its limits, then explicitly review a retry."
	default:
		return nil
	}
	if !readOnly {
		result.Repair = "Inspect original actions and receipts before retrying; this task already attempted work that may have effects."
	}
	return result
}
