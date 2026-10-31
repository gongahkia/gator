package executiontarget

import "github.com/gongahkia/gator/internal/action"

type Delivery string

const (
	NotDispatched        Delivery = "not_dispatched"
	DispatchAcknowledged Delivery = "dispatch_acknowledged"
	OutcomeUnknown       Delivery = "outcome_unknown"
)

// DeliveryFromAction projects existing receipt semantics without a second store.
// Acknowledgement is dispatch evidence, never proof of the intended terminal
// effect. Failed is conservatively unknown: failure alone does not prove that
// no bytes were sent. Use NotDispatched only with pre-dispatch rejection evidence.
// An unsettled actionlog entry already uses action.Unknown before dispatch.
func DeliveryFromAction(status action.Status) Delivery {
	switch status {
	case action.Pending, action.Denied:
		return NotDispatched
	case action.Executed:
		return DispatchAcknowledged
	default:
		return OutcomeUnknown
	}
}
