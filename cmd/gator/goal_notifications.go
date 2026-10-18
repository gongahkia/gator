package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gongahkia/gator/internal/goal"
)

const goalNotificationsUsage = "usage: gator goal notifications ID [--desktop on|off] [--urgent-only on|off] [--quiet HH:MM-HH:MM --timezone AREA/CITY | --clear-quiet]"

func goalNotificationsCommand(store goal.Store, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(goalNotificationsUsage)
	}
	record, err := store.Load(args[0])
	if err != nil {
		return err
	}
	if len(args) == 1 {
		_, err := fmt.Fprintf(out, "%s\nDesktop: %s (when globally enabled)\nQuiet hours: %s\nInbox: always retained\n", record.ID, goalDesktopStatus(record.Notifications), goalQuietStatus(record.Notifications))
		return err
	}
	flags := flag.NewFlagSet("goal notifications", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	desktop := flags.String("desktop", "", "on or off when desktop notifications are globally enabled")
	urgentOnly := flags.String("urgent-only", "", "desktop only for urgent alerts and failures")
	quiet := flags.String("quiet", "", "local quiet interval HH:MM-HH:MM")
	timezone := flags.String("timezone", "", "IANA timezone for quiet hours")
	clearQuiet := flags.Bool("clear-quiet", false, "remove quiet hours")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if len(flags.Args()) != 0 || *desktop == "" && *urgentOnly == "" && *quiet == "" && !*clearQuiet || *quiet != "" && *clearQuiet {
		return errors.New(goalNotificationsUsage)
	}
	policy := record.Notifications
	switch *desktop {
	case "":
	case "on":
		policy.DesktopMuted = false
	case "off":
		policy.DesktopMuted = true
	default:
		return errors.New("--desktop must be on or off")
	}
	switch *urgentOnly {
	case "":
	case "on":
		policy.UrgentOnly = true
	case "off":
		policy.UrgentOnly = false
	default:
		return errors.New("--urgent-only must be on or off")
	}
	if *clearQuiet {
		if *timezone != "" {
			return errors.New("--timezone cannot be combined with --clear-quiet")
		}
		policy.QuietStart, policy.QuietEnd, policy.Timezone = "", "", ""
	} else if *quiet != "" {
		parts := strings.Split(*quiet, "-")
		if len(parts) != 2 || *timezone == "" {
			return errors.New("--quiet requires HH:MM-HH:MM and --timezone")
		}
		policy.QuietStart, policy.QuietEnd, policy.Timezone = parts[0], parts[1], *timezone
	} else if *timezone != "" {
		return errors.New("--timezone requires --quiet")
	}
	record, err = store.SetNotificationPolicy(record.ID, policy)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "%s\nDesktop: %s (when globally enabled)\nQuiet hours: %s\nInbox: always retained\n", record.ID, goalDesktopStatus(record.Notifications), goalQuietStatus(record.Notifications))
	return err
}

func goalDesktopStatus(policy goal.NotificationPolicy) string {
	if policy.DesktopMuted {
		return "off"
	}
	if policy.UrgentOnly {
		return "on (urgent only)"
	}
	return "on"
}

func goalQuietStatus(policy goal.NotificationPolicy) string {
	if policy.QuietStart == "" {
		return "none"
	}
	return policy.QuietStart + "-" + policy.QuietEnd + " " + policy.Timezone
}

func goalAllowsDesktopNotification(stateDir, goalID string, now time.Time, urgent bool) bool {
	if goalID == "" {
		return true
	}
	store, err := goal.Open(stateDir)
	if err != nil {
		return false
	}
	record, err := store.Load(goalID)
	return err == nil && record.Notifications.AllowsDesktopUrgency(now, urgent)
}
