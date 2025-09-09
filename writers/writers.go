package writers

import (
	"io"

	"privdump/types"
)

func Write_file(in *types.Savedata, out io.Writer) {

	chunk_count := len(in.Forms) + len(in.Strings) + len(in.Blobs)
	length := 4 * (1 + chunk_count) //header length
	for c := range chunk_count {
		length += in.Chunk_length(c)
	}

	write_uint32_le(out, length)
	chunk_location := 4 * (1 + chunk_count)
	for c := range chunk_count {
		write_uint16_le(out, chunk_location)
		out.Write([]byte{0x00, 0xE0})
		chunk_location += in.Chunk_length(c)
	}

	out.Write(in.Blobs[types.OFFSET_SHIP])
	out.Write(in.Blobs[types.OFFSET_PLOT])
	out.Write(in.Blobs[types.OFFSET_MISSIONS])

	for i := range 3 {
		off := types.OFFSET_MISSION_BASE + 2*i
		if in.Strings[off] != "" {
			write_string_padded(out, in.Strings[off], in.Chunk_length(off))
			off = types.OFFSET_MISSION_BASE + 2*i + 1
			write_form(out, in.Forms[off])
		}
	}

	write_form(out, in.Forms[types.OFFSET_PLAY])
	out.Write(in.Blobs[types.OFFSET_WTF])
	write_form(out, in.Forms[types.OFFSET_SSSS])
	write_form(out, in.Forms[types.OFFSET_REAL])
	write_string_padded(out, in.Strings[types.OFFSET_NAME], in.Chunk_length(types.OFFSET_NAME))
	write_string_padded(out, in.Strings[types.OFFSET_CALLSIGN], in.Chunk_length(types.OFFSET_CALLSIGN))
}

func write_form(out io.Writer, form *types.Form) {
	write_string_padded(out, "FORM", 4)
	write_uint32_be(out, form.Real_size()-8)
	write_string_padded(out, form.Name, 4)

	sub := 0
	for r, record := range form.Records {
		if record.Name == "FORM" {
			write_form(out, form.Subforms[sub])
			sub += 1
			continue
		}
		write_string_padded(out, record.Name, len(record.Name))
		write_uint32_be(out, len(record.Data))
		out.Write(record.Data)

		// Omitting the footer will break things.
		// I believe footers exist to pad the record size out to an even number,
		// (Just in case that seems to make sense, note that it causes every form and record to be misaligned (not on a 2-byte barrier) in the file data)
		if record.Needs_footer() {
			// Footer content should be the same as the next byte; this is ridiculous but it's what Privateer does.
			if r != len(form.Records)-1 {
				out.Write([]byte{form.Records[r+1].Name[0]})
			} else {
				// Next byte is not available, so re-write the last byte.
				//fmt.Println("Doubling last byte in", record.Name, form.Name)
				out.Write(record.Data[len(record.Data)-1 : len(record.Data)])
			}
		}
	}
}

// The primitives that actually write data

// write_string_padded writes a variable-length string into a fixed-length slot
// unneeded bytes are padded out with 0s.
func write_string_padded(out io.Writer, str string, length int) (int, error) {
	return out.Write(append([]byte(str), make([]byte, length-len(str))...))
}

func write_uint32_be(out io.Writer, i int) (int, error) {
	// Ugh.  Is this really what bit twiddling has to look like in Go?
	return out.Write([]byte{uint8(i >> 24), uint8((i >> 16) & 0xff), uint8((i >> 8) & 0xff), uint8(i & 0xff)})
}

func write_uint32_le(out io.Writer, i int) (int, error) {
	return out.Write([]byte{uint8(i & 0xff), uint8((i >> 8) & 0xff), uint8((i >> 16) & 0xff), uint8(i >> 24)})
}

func write_uint16_le(out io.Writer, i int) (int, error) {
	return out.Write([]byte{uint8(i & 0xff), uint8((i >> 8) & 0xff)})
}
