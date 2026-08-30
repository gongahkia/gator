package catalog

type Entry struct {
	ID      string
	Name    string
	Enabled bool
}

type Patch struct {
	ID      string
	Name    *string
	Enabled *bool
}
