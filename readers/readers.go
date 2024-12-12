package readers

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"privdump/types"
)

func Read_string(bytes []byte, cur *int) (string, int, error) {
	//reads null-terminated string,  Advances the cursor past the terminating null.
	old := *cur
	for ; ; *cur += 1 {
		if *cur >= len(bytes) {
			*cur = old
			return "", 0, errors.New("Out of space")
		}

		if bytes[*cur] == 0 {
			return string(bytes[old:*cur]), 0, nil
		}
	}

}

func Read_fixed_string(target string, bytes []byte, cur *int) (int, error) {
	tb := []byte(target)
	ltb := len(tb)
	maxx := len(bytes) - ltb - *cur

	for i := 0; i < maxx; i += 1 {
		if slices.Equal(bytes[*cur+i:*cur+i+ltb], tb) {
			*cur += (i + ltb)
			return 0, nil
		}
	}

	return 0, errors.New("Could not find string " + target)
}

func Read_uint8(bytes []byte, cur *int) uint8 {
	out := bytes[*cur]
	*cur += 1
	return out
}

func Read_fixed_uint8(bytes []byte, cur *int, expected uint8) error {
	out := bytes[*cur]
	*cur += 1
	if out != expected {
		return errors.New(fmt.Sprintf("Expected %v; read %v", expected, out))
	}
	return nil
}

func Read_int(bytes []byte, cur *int) int {
	// big-endian
	out := uint(0)
	for _ = range 4 {
		out = out << 8
		out = out + uint(bytes[*cur])
		*cur += 1
	}

	return int(out)
}

func Read_int_le(bytes []byte, cur *int) int {
	// little-endian
	out := uint(0)
	for i := range 4 {
		out = out + uint(bytes[*cur])<<(8*i)
		*cur += 1
	}

	return int(out)
}

func Read_int16(bytes []byte, cur *int) int {
	// little-endian!
	out := int(0)
	for i := range 2 {
		out = out + (int(uint(bytes[*cur])) << (8 * i))
		*cur += 1
	}
	if out > 0x8000 {
		out -= 0x10000
	}

	return out
}

func safe_lookup[K comparable](from map[K]string, with K) string {
	out, ok := from[with]
	if !ok {
		out = fmt.Sprintf("Unknown (%v)", with)
	}
	return out
}

func Read_header(in []byte) types.Header {
	//Header format:
	//
	// bytes 0x00-0x03: File size
	// bytes 0x04-??: Offsets
	//   Offsets are locations of things in the save file.  It is odd to see these in a save file format - perhaps it is also a memory dump?
	//   Each offset is 4 bytes.  Technically, only the first 2 bytes are the location; the last 2 bytes are always 00E0.  Maybe it's some sort of thunk?
	//   The number of offsets varies.  The named 9 in the offset enum are always present, but there are 2 more for each non-plot mission
	//   This number can be determined by peeking where the MISSIONS offset points.  (Or by caculating based on the first offset?  Are we sure there's never a footer??)
	//   We currently peek, which means we can't properly read the header without reading one byte from the body.
	// bytes ??-?? : footer
	//   That which lies between the offset block and the first offset.
	out := types.Header{}

	cur := 0
	out.File_size = Read_int16(in, &cur)
	cur += 2
	for i := 0; i <= types.OFFSET_MISSIONS; i += 1 {
		out.Offsets = append(out.Offsets, Read_int16(in, &cur))
		cur += 2
	}

	// Peek at the data  (TODO: or use offset[0])
	cur2 := out.Offsets[types.OFFSET_MISSIONS]
	missions := Read_int16(in, &cur2)

	// Expect 2 more offsets for each missions
	for i := 0; i < 2*missions; i += 1 {
		out.Mission_offsets = append(out.Mission_offsets, Read_int16(in, &cur))
		cur += 2
	}

	for i := types.OFFSET_MISSIONS + 1; i < types.OFFSET_COUNT; i += 1 {
		out.Offsets = append(out.Offsets, Read_int16(in, &cur))
		cur += 2
	}

	out.Footer = in[cur:out.Offsets[0]]
	cur = out.Offsets[0]

	return out
}

func Read_form(bytes []byte, cur *int) (types.Form, error) {
	// Form format:
	//
	// 1 Identifier: A 4-byte capital-letter string which is always "FORM"
	// 2 Data Length: 4 bytes indicating the length of the data (big endian int, presumably unsigned).
	// Data:
	//    3 Form name: 4-byte capital-letter string
	//    4 0 or more records.
	//   (5) A possible "footer", which is any leftover bytes claimed by the length but not actually containing a record
	//        This is usually part of something else and may indicate that length is a "Read at least this much" type of suggestion
	//
	// Note that the length does not include the length of the name or of the length itself.

	// Record Format:
	//
	// 1 Name: 4-byte capital-letter string
	// 2 Data Length: 4 bytes indicating the length of the data (big endian int, presumably unsigned).
	// 3 Data: could be anything, but there is one very special case:  If the name is "FORM" then this record is a form, and so the data is a form name plus a list of records.
	//
	// Again, length does not include the first 8 bytes

	out := types.Form{}

	_, err := Read_fixed_string("FORM", bytes, cur)
	if err != nil {
		return out, err
	}

	out.Length = Read_int(bytes, cur)
	form_start := *cur
	form_end := form_start + out.Length

	out.Name = string(bytes[*cur : *cur+4])
	*cur += 4

	for *cur <= form_end-8 { // Minimum record size is 8

		// This is here to pass over an observed 0 between records in a form in priv.tre.
		for bytes[*cur] == 0 {
			*cur += 1
		}

		record_name, _, err := Read_string(bytes[:form_end], cur)
		if err != nil {
			fmt.Println("Unable to read record")
			fmt.Println(fmt.Sprintf("Ignoring %v footer at %v: %v", out.Name, *cur, bytes[*cur:form_end]))
			*cur = form_end
			break
		}
		length := Read_int(bytes, cur)
		record_start := *cur
		//fmt.Println(fmt.Sprintf("Record %v  %v->%v", record_name, *cur, *cur+length))

		record := types.Record{record_name, bytes[*cur : *cur+length]}

		if strings.HasSuffix(record_name, "FORM") {
			*cur -= 8 // EVIL HACK!! go back and re-parse this record as a form.
			// Subforms!!!
			for *cur < record_start+length {
				form, err := Read_form(bytes, cur)
				if err != nil {
					//fmt.Println(bytes[*cur:record_start+length])
					//*cur = record_start+length
					break
				}

				//fmt.Println("Adding", form.Name, "to", out.Name)
				out.Subforms = append(out.Subforms, form)
			}

		} else {
			*cur += length
		}

		//fmt.Println("Adding", record_name, "to", out.name)
		out.Records = append(out.Records, record)
	}

	out.Footer = bytes[*cur:form_end]
	*cur = form_end

	return out, nil
}
