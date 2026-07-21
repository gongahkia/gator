package privacy

import "errors"

var ErrTelemetryDisabled = errors.New("telemetry is disabled")

const TelemetryEnabled = false

var ForbiddenTelemetryImportPrefixes = []string{
	"go.opentelemetry.io/",
	"github.com/getsentry/",
	"github.com/segmentio/analytics",
	"github.com/posthog/",
	"github.com/amplitude/",
	"github.com/mixpanel/",
}

func CheckTelemetry() error {
	return ErrTelemetryDisabled
}
