package attachment

// Original is an explicitly selected input's original bytes, before document
// extraction or provider adaptation. It stays host-private and is never a model
// Attachment. The Work host assigns an ephemeral reference for exact review.
type Original struct {
	Name string
	Data []byte `json:"-"`
}
