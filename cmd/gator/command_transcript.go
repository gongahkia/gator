package main

import (
	"errors"
	"fmt"
	"html"
	"io"
	"strings"

	"github.com/gongahkia/gator/internal/agent"
	"github.com/gongahkia/gator/internal/journal"
)

// exportTranscript creates a portable, local HTML transcript. It never uploads
// session contents: sharing remains an explicit developer action after review.
func exportTranscript(arguments []string, out io.Writer) error {
	if len(arguments) != 1 {
		return errors.New("usage: gator transcript RUN_RECORD_PATH > transcript.html")
	}
	session, err := journal.LoadSession(arguments[0])
	if err != nil {
		return err
	}
	if _, err := io.WriteString(out, "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>Gator transcript</title><style>body{font-family:ui-monospace,Menlo,monospace;max-width:72rem;margin:2rem auto;padding:0 1rem;color:#202124}pre{white-space:pre-wrap;word-break:break-word;background:#f6f8fa;padding:1rem;border-radius:.4rem}.meta{color:#59636e}.user{border-left:4px solid #388bfd}.agent{border-left:4px solid #1a7f37}.tool{border-left:4px solid #bf8700}</style></head><body>\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "<h1>Gator transcript</h1><p class=\"meta\">Provider: %s<br>Model: %s<br>Thread: %s<br>Mode: %s</p>\n", escape(session.Provider), escape(session.Model), escape(session.ThreadID), escape(session.Mode)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "<h2>Task</h2><pre>%s</pre>\n", escape(session.Task)); err != nil {
		return err
	}
	for _, message := range session.Messages {
		role, class := transcriptRole(message.Role)
		content := message.Content
		if len(message.ToolCalls) > 0 {
			names := make([]string, 0, len(message.ToolCalls))
			for _, call := range message.ToolCalls {
				names = append(names, call.Name)
			}
			if content != "" {
				content += "\n\n"
			}
			content += "Tool calls: " + strings.Join(names, ", ")
		}
		if message.Role == agent.RoleTool && message.ToolName != "" {
			content = "Tool: " + message.ToolName + "\n" + content
		}
		if _, err := fmt.Fprintf(out, "<section class=\"%s\"><h2>%s</h2><pre>%s</pre></section>\n", class, role, escape(content)); err != nil {
			return err
		}
	}
	_, err = io.WriteString(out, "</body></html>\n")
	return err
}

func transcriptRole(role agent.Role) (string, string) {
	switch role {
	case agent.RoleUser:
		return "Developer", "user"
	case agent.RoleAgent:
		return "Gator", "agent"
	case agent.RoleTool:
		return "Tool result", "tool"
	default:
		return "Message", "meta"
	}
}

func escape(value string) string { return html.EscapeString(value) }
