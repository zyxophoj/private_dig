package readers

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"

	"privdump/types"
)

func read_fixed(r io.Reader, size int) ([]byte, error) {
	into := make([]byte, size)
	n, err := r.Read(into)
	if err != nil {
		return nil, err
	}
	if n != size {
		return nil, errors.New(fmt.Sprintf("Failed to read ^b bytes (got only %v)", size, n))
	}

	return into, nil
}

// Advance advances a reader forwards
// It's a forward-only seek for something that doesn't seek. 
func Advance(r io.Reader, size int) error {
	_, err := read_fixed(r, size)
	return err
}

func Read_string(r io.Reader) (string, int, error) {
	//reads null-terminated string,  Advances the reader past the terminating null.
	one := []byte{0}
	out := []byte{}

	for {
		n, err := r.Read(one)
		if err != nil {
			return "", 0, err
		}
		if n != 1 {
			return "", 0, errors.New("failed to read")
		}

		if one[0] == 0 {
			return string(out), len(out) + 1, nil
		}

		out = append(out, one[0])
	}
}

func Read_fixed_string(target string, r io.Reader) (int, error) {
	target_buf := []byte(target)
	read_buf, err := read_fixed(r, len(target_buf))

	if err != nil {
		return 0, err
	}

	if !slices.Equal(read_buf, target_buf) {
		return 0, errors.New("Could not find string " + target + "(got " + string(read_buf) + ")")
	}

	return len(read_buf), nil
}

func Read_int_be(r io.Reader) (int, error) {
	bytes, err := read_fixed(r, 4)
	if err != nil {
		return 0, err
	}
	// big-endian
	out := uint(0)
	for cur := range 4 {
		out = out << 8
		out = out + uint(bytes[cur])
	}

	return int(out), nil
}

func Read_int_le(r io.Reader) (int, error) {
	bytes, err := read_fixed(r, 4)
	if err != nil {
		return 0, err
	}
	// little-endian
	out := uint(0)
	for cur := range 4 {
		out = out + uint(bytes[cur])<<(8*cur)
	}

	return int(out), nil
}

func Read_int16(r io.Reader) (int, error) {
	bytes, err := read_fixed(r, 2)
	if err != nil {
		return 0, err
	}
	// little-endian
	out := int(0)
	for cur := range 2 {
		out = out + (int(uint(bytes[cur])) << (8 * cur))
	}
	if out > 0x8000 {
		out -= 0x10000
	}

	return int(out), nil
}

func read_header(r io.Reader) (types.Header, error) {
	//Header format:
	//
	// bytes 0x00-0x03: File size
	// bytes 0x04-??: Offsets
	//   Offsets are locations of things in the save file.  It is odd to see these in a save file format - perhaps it is also a memory dump?
	//   Each offset is 4 bytes.  Technically, only the first 2 bytes are the location; the last 2 bytes are always 00E0.  Maybe it's some sort of thunk?
	//   The number of offsets varies.  The named 9 in the offset enum are always present, but there are 2 more for each non-plot mission
	//   This number can be determined by peeking where the MISSIONS offset points, or by caculating based on the first offset.
	out := types.Header{}

	size, err := Read_int_le(r)
	if err != nil {
		return out, err
	}
	out.File_size = size

	for i := 0; i <= types.OFFSET_MISSIONS; i += 1 {
		offset, err := Read_int16(r)
		if err != nil {
			return out, err
		}
		out.Offsets = append(out.Offsets, offset)
		Advance(r, 2)
	}

	// Data starts where offsets end, so offset[0] indirectly tells us how many offsets there are.
	// The -1 is for the file size.
	missions := (out.Offsets[0]/4 - types.OFFSET_COUNT - 1) / 2

	// Expect 2 more offsets for each mission
	mission_offsets := []int{}
	for i := 0; i < 2*missions; i += 1 {
		offset, err := Read_int16(r)
		if err != nil {
			return out, err
		}
		mission_offsets = append(mission_offsets, offset)
		Advance(r, 2)
	}

	for i := types.OFFSET_MISSIONS + 1; i < types.OFFSET_COUNT; i += 1 {
		offset, err := Read_int16(r)
		if err != nil {
			return out, err
		}
		out.Offsets = append(out.Offsets, offset)
		Advance(r, 2)
	}

	out.Offsets = append(out.Offsets, mission_offsets...)

	// TODO: advance to offsets[0]?

	return out, nil
}

// Read_savedata reads savedata (presumably, from a Privateer/RF savefile)
func Read_savedata(r io.ReadSeeker) (*types.Savedata, error) {
	header, err := read_header(r)
	if err != nil {
		return nil, err
	}
	out := types.Savedata{
		Forms:   map[int]*types.Form{},
		Strings: map[int]string{},
		Blobs:   map[int][]byte{},
	}

	for true_index, _ := range header.Offsets {
		i := types.Modify_index(true_index, header.Missions())

		// An attempt was made to use only io.Reader for file reading.
		// It failed because mission forms sometimes lie about their lengths in a manner which claims the
		// first byte of the next chunk.  This means we can not rely on being at the start of chunk n+1 after reading chunk n.
		// This bullshit could in principle be caught and worked around, but it would massively complicate form reading.
		r.Seek(int64(header.Offsets[i]), io.SeekStart)
		switch header.Chunk_type(i) {
		case types.CT_FORM:
			f, err := Read_form(r)
			if err != nil {
				return nil, fmt.Errorf("Failed to load form at offset %v - %v", i, err)
			}
			out.Forms[i] = f

		case types.CT_STRING:
			str, n, err := Read_string(r)
			if err != nil {
				return nil, errors.New("Failed to read string")
			}
			out.Strings[i] = str
			// Variable-length string in fixed-length chunk
			chunk_length := header.Offset_end(i) - header.Offsets[i]
			Advance(r, chunk_length-n)

		case types.CT_BLOB:
			blob, err := read_fixed(r, chunk_length)
			if err != nil {
				return nil, errors.New("Failed to read blob")
			}
			out.Blobs[i] = blob
		}
	}

	return &out, nil
}

func Read_form(r io.Reader) (*types.Form, error) {
	// Form format:
	//
	// 1 Identifier: A 4-byte capital-letter string which is always "FORM"
	// 2 Data Length: 4 bytes indicating the length of the data (big endian int, presumably unsigned).
	// Data:
	//    3 Form name: 4-byte capital-letter string
	//    4 0 or more records.
	//    5  A possible footer - anything within the form length that is not claimed by any records
	//       (This only seems to happen for mission data forms and is probably just caused by the game calculating the length incorrectly)
	// Note that the length does not include the length of the identifier "FORM" or of the length itself.

	// Record Format:
	//
	// 1 Name: 4-byte capital-letter string
	// 2 Data Length: 4 bytes indicating the length of the data (big endian int, presumably unsigned).
	// 3 Data: could be anything, but there is one very special case:  If the name is "FORM" then this record is a form, and so the data is a form name plus a list of records.
	// (4) A possible "footer" - this is one byte which pads the record out to an even length, and therefore only appears if the "length" field is odd.
	//
	// Again, length does not include the first 8 bytes or any footer.

	_, err := Read_fixed_string("FORM", r)
	if err != nil {
		return nil, err
	}

	length, err := Read_int_be(r)
	if err != nil {
		return nil, err
	}

	return read_form_inner(r, length)
}

func read_form_inner(r io.Reader, length int) (*types.Form, error) {
	name_buf, err := read_fixed(r, 4)
	if err != nil {
		return nil, err
	}
	bytes_read := 4
	out := types.Form{Length: length, Name: string(name_buf)}

	// records
	for bytes_read <= length-8 { // Minimum record size is 8
		record_name_buf, err := read_fixed(r, 4)
		if err != nil {
			fmt.Println("Unable to read record")
			//fmt.Println(fmt.Sprintf("Ignoring %v footer at %v: %v", out.Name, *cur, bytes[*cur:form_end]))
			break
		}
		bytes_read += 4

		length, err := Read_int_be(r)
		bytes_read += 4

		record_bytes, err := read_fixed(r, length)
		bytes_read += length

		//fmt.Println(fmt.Sprintf("Record %v  %v->%v", record_name, *cur, *cur+length))

		record := types.Record{string(record_name_buf), record_bytes, nil}
		if length%2 == 1 {
			record.Footer, _ = read_fixed(r, 1)
			bytes_read += 1
		}
		//fmt.Println("Adding", record_name, "to", out.name)
		out.Records = append(out.Records, &record)

		if record.Name == "FORM" {
			// This record is a form.
			// We keep the record but also re-read the record data as form data
			form, err := read_form_inner(bytes.NewReader(record.Data), len(record.Data))
			if err != nil {
				//fmt.Println(bytes[*cur:record_start+length])
				//*cur = record_start+length
				break
			}

			//fmt.Println("Adding", form.Name, "to", out.Name)
			out.Subforms = append(out.Subforms, form)
		}
	}

	// Any extra bytes left (there shouldn't be, but sometimes are)
	out.Footer, _ = read_fixed(r, length-bytes_read)

	return &out, nil
}
