package mcp

func toolDefinitions() []Tool {
	return []Tool{
		{
			Name:        "gather",
			Title:       "Gather Raw Context",
			Description: "Run paw gather for cwd and instruction, returning an Envelope with raw context.",
			InputSchema: objectSchema(map[string]any{
				"cwd":         stringSchema("Working directory to inspect."),
				"instruction": stringSchema("Task instruction used to gather relevant context."),
			}, []string{"cwd", "instruction"}),
		},
		{
			Name:        "compress",
			Title:       "Compress Context",
			Description: "Run paw compress for an Envelope or raw context, returning a validated compressed Envelope.",
			InputSchema: map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"properties": map[string]any{
					"cwd":         stringSchema("Working directory for raw context input."),
					"instruction": stringSchema("Task instruction for raw context input."),
					"envelope":    map[string]any{"type": "object", "description": "paw Envelope with raw context."},
					"raw":         map[string]any{"type": "object", "description": "paw RawContext object."},
					"include_raw": boolSchema("Return the full Envelope including raw context."),
				},
				"oneOf": []map[string]any{
					{"required": []string{"envelope"}},
					{"required": []string{"cwd", "instruction", "raw"}},
				},
			},
		},
		{
			Name:        "digest",
			Title:       "Gather And Compress Context",
			Description: "Run paw gather plus compress for cwd and instruction, returning a validated digest Envelope.",
			InputSchema: objectSchema(map[string]any{
				"cwd":         stringSchema("Working directory to inspect."),
				"instruction": stringSchema("Task instruction used for gather and compression."),
				"include_raw": boolSchema("Return the full Envelope including raw context."),
			}, []string{"cwd", "instruction"}),
		},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties":           properties,
		"required":             required,
	}
}

func stringSchema(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func boolSchema(description string) map[string]any {
	return map[string]any{"type": "boolean", "description": description}
}
