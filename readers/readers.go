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
			*cur += 1 // Advance past null
			return string(bytes[old : *cur-1]), 0, nil
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
	//   This number can be determined by peeking where the MISSIONS offset points, or by caculating based on the first offset.
	out := types.Header{}

	cur := 0
	out.File_size = Read_int16(in, &cur)
	cur += 2
	for i := 0; i <= types.OFFSET_MISSIONS; i += 1 {
		out.Offsets = append(out.Offsets, Read_int16(in, &cur))
		cur += 2
	}

	// Data starts where offsets end, so offset[0] indirectly tells us how many offsets there are.
	// The -1 is for the file size.
	missions := (out.Offsets[0]/4-types.OFFSET_COUNT-1)/2

	// Expect 2 more offsets for each mission
	mission_offsets := []int{}
	for i := 0; i < 2*missions; i += 1 {
		mission_offsets = append(mission_offsets, Read_int16(in, &cur))
		cur += 2
	}

	for i := types.OFFSET_MISSIONS + 1; i < types.OFFSET_COUNT; i += 1 {
		out.Offsets = append(out.Offsets, Read_int16(in, &cur))
		cur += 2
	}

	out.Offsets = append(out.Offsets, mission_offsets...)

	cur = out.Offsets[0]

	return out
}

func Read_savedata(header types.Header, bytes []byte) (*types.Savedata, error) {
	out := types.Savedata{
		Forms:   map[int]*types.Form{},
		Strings: map[int]string{},
		Blobs:   map[int][]byte{},
	}

	off_forms := []int{types.OFFSET_PLAY, types.OFFSET_SSSS, types.OFFSET_REAL}
	off_strings := []int{types.OFFSET_NAME, types.OFFSET_CALLSIGN}
	off_blobs := []int{types.OFFSET_SHIP, types.OFFSET_PLOT, types.OFFSET_MISSIONS, types.OFFSET_WTF}
	for i := range (len(header.Offsets) - types.OFFSET_COUNT) / 2 {
		off_strings = append(off_blobs, types.OFFSET_MISSION_BASE+2*i)
		off_forms = append(off_forms, types.OFFSET_MISSION_BASE+2*i+1)
	}

	//TODO:: switch?
	for _, i := range off_forms {
		cur := header.Offsets[i]
		f, err := Read_form(bytes, &cur)
		if err != nil {
			return nil, fmt.Errorf("Failed to load form %v - %v", i, err)
		}
		out.Forms[i] = &f
	}
	for _, i := range off_strings {
		cur := header.Offsets[types.OFFSET_NAME]
		str, _, err := Read_string(bytes, &cur)
		if err != nil {
			return nil, errors.New("Failed to read string")
		}
		out.Strings[i] = str
	}
	for _, i := range off_blobs {
		out.Blobs[i] = bytes[header.Offsets[i]:header.Offset_end(i)]
	}

	return &out, nil
}

func Read_form(bytes []byte, cur *int) (types.Form, error) {
	// Form format:
	//
	// 1 Identifier: A 4-byte capital-letter string which is always "FORM"
	// 2 Data Length: 4 bytes indicating the length of the data (big endian int, presumably unsigned).
	// Data:
	//    3 Form name: 4-byte capital-letter string
	//    4 0 or more records.
	//
	// Note that the length does not include the length of the identifier "FORM" or of the length itself.

	// Record Format:
	//
	// 1 Name: 4-byte capital-letter string
	// 2 Data Length: 4 bytes indicating the length of the data (big endian int, presumably unsigned).
	// 3 Data: could be anything, but there is one very special case:  If the name is "FORM" then this record is a form, and so the data is a form name plus a list of records.
	// (4) A possible "footer", - this is one byte which pads the record out to an even length, and therefore only appear if the "length field is odd.
	//
	// Again, length does not include the first 8 bytes or any footer.

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
		record_name, _, err := Read_string(bytes[:form_end], cur)
		if err != nil {
			fmt.Println("Unable to read record")
			fmt.Println(fmt.Sprintf("Ignoring %v footer at %v: %v", out.Name, *cur, bytes[*cur:form_end]))
			*cur = form_end
			break
		}
		// We just read a fixed-length string as if it was null-terminated.
		// we get away with this because an int (record length) follows it, and this is unlikel to use the top byte.
		// TODO: implement and use read_fixed_length_string
		*cur -= 1

		length := Read_int(bytes, cur)
		record_start := *cur
		//fmt.Println(fmt.Sprintf("Record %v  %v->%v", record_name, *cur, *cur+length))

		record := types.Record{record_name, bytes[*cur : *cur+length], nil}

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
				out.Subforms = append(out.Subforms, &form)
			}

		} else {
			*cur += length
		}

		if length%2 == 1 {
			record.Footer = []byte{Read_uint8(bytes, cur)}
		}

		//fmt.Println("Adding", record_name, "to", out.name)
		out.Records = append(out.Records, &record)
	}

	if *cur != form_end {
		// form-footer?
		// I don't think this can happen, but would like to know if it does.
		fmt.Println("EXTRA BYTES AT END Of FORM:", bytes[*cur:form_end])
		*cur = form_end
	}

	return out, nil
}
