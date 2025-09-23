package writers

// Functions for writing to a file
// This is dwindling to nothing now that polymorphic clases know how to write themselves.

import (
	"io"
)

// write_string_padded writes a variable-length string into a fixed-length slot
// unneeded bytes are padded out with 0s.
func Write_string_padded(out io.Writer, str string, length int) (int, error) {
	return out.Write(append([]byte(str), make([]byte, length-len(str))...))
}

func Write_uint32_be(out io.Writer, i int) (int, error) {
	// Ugh.  Is this really what bit twiddling has to look like in Go?
	return out.Write([]byte{uint8(i >> 24), uint8((i >> 16) & 0xff), uint8((i >> 8) & 0xff), uint8(i & 0xff)})
}

func Write_uint32_le(out io.Writer, i int) (int, error) {
	return out.Write([]byte{uint8(i & 0xff), uint8((i >> 8) & 0xff), uint8((i >> 16) & 0xff), uint8(i >> 24)})
}

func Write_uint16_le(out io.Writer, i int) (int, error) {
	return out.Write([]byte{uint8(i & 0xff), uint8((i >> 8) & 0xff)})
}
