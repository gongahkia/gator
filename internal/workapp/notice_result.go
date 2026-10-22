package workapp

import (
	"strings"

	"github.com/gongahkia/gator/internal/uisnapshot"
)

func noticeMessage(title, text, tone string) message {
	return message{role: "Gator", text: text, result: &uisnapshot.Result{Kind: "notice", Title: title, Tone: tone, Summary: text}}
}

func reportMessage(title, text, tone string) message {
	if !strings.Contains(text, "\n") {
		return noticeMessage(title, text, tone)
	}
	return message{role: "Gator", text: text, result: &uisnapshot.Result{Kind: "report", Title: title, Tone: tone, Lines: strings.Split(strings.TrimSpace(text), "\n")}}
}
