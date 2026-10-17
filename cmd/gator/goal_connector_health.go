package main

import (
	"fmt"
	"strings"

	"github.com/gongahkia/gator/internal/action"
	"github.com/gongahkia/gator/internal/connector"
	"github.com/gongahkia/gator/internal/jobs"
)

// goalConnectorReadiness reports configured access without making network
// requests or exposing tokens. A successful status is not a reachability test.
func goalConnectorReadiness(definition jobs.Definition) []string {
	if len(definition.ConnectorIDs) == 0 {
		return nil
	}
	settings, err := loadSettings()
	if err != nil {
		return []string{"connector settings unavailable; inspect gator provider connector list"}
	}
	registry, err := connector.NewRegistry(settings.Connectors)
	if err != nil {
		return []string{"connector settings unavailable; inspect gator provider connector list"}
	}
	credentials, err := gatorCredentials()
	if err != nil {
		return []string{"connector credentials unavailable; inspect gator provider connector list"}
	}
	permissions := connector.PermissionSet(settings.ConnectorPermissions)
	var lines []string
	for index, id := range definition.ConnectorIDs {
		if index == 4 {
			lines = append(lines, fmt.Sprintf("and %d more selected connectors", len(definition.ConnectorIDs)-4))
			break
		}
		descriptor, found := registry.Get(id)
		if !found {
			lines = append(lines, fmt.Sprintf("%s: configuration missing; edit or disable this check", id))
			continue
		}
		status, err := connectorAuthStatus(descriptor, credentials)
		if err != nil {
			lines = append(lines, fmt.Sprintf("%s: credential status unavailable; inspect gator provider connector status %s", id, id))
			continue
		}
		var blocked []string
		if len(definition.WatchSources) > 0 {
			for _, source := range definition.WatchSources {
				if source.ConnectorID != id {
					continue
				}
				_, operation, err := registry.Operation(id, source.Operation)
				if err != nil || operation.Capability != action.ConnectedRead || permissions.Resolve(id, source.Operation, operation.Capability) != connector.PermissionAllow {
					blocked = append(blocked, source.Operation)
				}
			}
		} else {
			readable := false
			for _, operation := range descriptor.Operations() {
				if operation.Capability == action.ConnectedRead && permissions.Resolve(id, operation.ID, operation.Capability) == connector.PermissionAllow {
					readable = true
					break
				}
			}
			if !readable {
				blocked = append(blocked, "all reads")
			}
		}
		line := id + ": " + status
		if definition.OAuthAccountPins[id] != "" {
			line += "; account pinned; live identity checked when reading"
		} else if pin := definition.OAuthRefreshPins[id]; pin != "" {
			if pinErr := verifyGoalOAuthPin(id, pin, registry, credentials); pinErr != nil {
				line += "; " + pinErr.Error()
			}
		} else if descriptor.Kind == connector.KindGoogle && descriptor.Authentication == connector.AuthOAuth {
			line += "; account is not pinned for this older check; recreate it after confirming the account"
		}
		if len(blocked) > 0 {
			line += "; read permission missing for " + strings.Join(blocked, ", ")
		}
		if status == "login required" || status == "credential expired; login required" {
			line += "; run gator provider connector login " + id
		}
		lines = append(lines, line)
	}
	return lines
}
