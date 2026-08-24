package report

import (
	"io"

	"github.com/ojarosch/iacbom/internal/bom"
)

// JSON writes the stable machine-readable iacbom format.
func JSON(w io.Writer, b *bom.BOM) error {
	return writeJSON(w, b)
}
