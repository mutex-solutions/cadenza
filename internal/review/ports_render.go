package review

// Renderer turns short markdown into sanitized HTML. Raw HTML in the input is
// dropped and only safe link schemes are kept. Implementation: adapter/markdown.
type Renderer interface {
	Render(markdown string) string
}
