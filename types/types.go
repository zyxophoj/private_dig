package types

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"privdump/readers"
	"privdump/writers"
)

type Game int

const (
	GT_NONE Game = iota
	GT_PRIV
	GT_RF
)

// This is not the true chunk order.
// The game stores mission info between OFFSET_MISSIONS and OFFSET_PLAY,
// which means the true index of anything after OFFSET_MISSIONS is variable.
//
// We want names to mean things, so mission info is moved to the end

const (
	OFFSET_SHIP = iota // Ship type, location, guild membership
	OFFSET_PLOT        //Plot status
	OFFSET_MISSIONS
	OFFSET_PLAY // player kill count + reputation
	OFFSET_WTF
	OFFSET_SSSS // Hidden jump points?
	OFFSET_REAL // ship equipment
	OFFSET_NAME
	OFFSET_CALLSIGN

	OFFSET_COUNT                       // This only counts the always-present offsets
	OFFSET_MISSION_BASE = OFFSET_COUNT // Used as an internal index for the first non-plot mission
)

func Offset_name(o int) string {
	return []string{"ShipLocGuilds", "Plot", "Mission count", "Score", "WTF", "Hidden Jumps", "Equipment", "Name", "Callsign",
		"Mission 1 Name", "Mission 1 Data", "Mission 2 Name", "Mission 2 Data", "Mission 3 Name", "Mission 3 Data"}[o]
}

type ChunkType int

const (
	CT_BLOB   ChunkType = iota // A non-definition: a blob is just a sequence of bytes
	CT_FORM                    // See the form class
	CT_STRING                  // null-terminated string, padded out with 0 or more nulls to some fixed length
)

type Header struct {
	File_size int
	Offsets   []int
}

func (h *Header) Missions() int {
	// h.Offsets[0] is where non-offsets start
	// we subtract 1 because offsets start after the file size, and subtrace OFFSET_COUNT to count just the mission chunks.
	return (h.Offsets[0]/4 - OFFSET_COUNT - 1) / 2
}

// Modify_index converts a true index into an OFFSET_* enum value
func Modify_index(i int, missions int) int {
	if i > OFFSET_MISSIONS && i <= OFFSET_MISSIONS+2*missions {
		// First mission starts at OFFSET_MISSIONS+1;
		i = i - (OFFSET_MISSIONS + 1) + OFFSET_MISSION_BASE
	} else {
		if i > OFFSET_MISSIONS+2*missions {
			i -= 2 * missions
		}
	}

	return i
}

func (h *Header) Chunk_type(i int) ChunkType {
	if i >= OFFSET_MISSION_BASE {
		// This is mission-related
		if (i-OFFSET_MISSION_BASE)%2 == 0 {
			return CT_STRING
		}
		return CT_FORM
	}

	switch i {
	case OFFSET_PLAY, OFFSET_SSSS, OFFSET_REAL:
		return CT_FORM
	case OFFSET_NAME, OFFSET_CALLSIGN:
		return CT_STRING
	case OFFSET_SHIP, OFFSET_PLOT, OFFSET_MISSIONS, OFFSET_WTF:
		return CT_BLOB
	}

	// for the compiler.  This shouldn't happen
	return CT_BLOB
}

// Offset_end returns the index of the byte one after the end of the offset with the given offset ID.
// This will normally be h.Offsets[o+1], but enum order not matching file order (due to moving missions to the end of the list) complicates things.
func (h *Header) Offset_end(o int) int {
	if o == OFFSET_MISSIONS && len(h.Offsets) > OFFSET_COUNT {
		// Jump to mission offsets
		return h.Offsets[OFFSET_MISSION_BASE]
	}
	if h.Missions() > 0 && o == len(h.Offsets)-1 {
		// At the end of mission offsets, jump back
		return h.Offsets[OFFSET_PLAY]
	}
	if o == OFFSET_CALLSIGN {
		// Since this is the last offset, we can't look at the next one to see where it ends.
		return h.File_size
	}

	return h.Offsets[o+1]
}

// Savedata stores mostly-parsed data from a savefile
// Data at each offset fits into one of 3 categories:
// Form: see the Form class
// String: null-terminated string, inside a fixed-length space with 0 or more extra nulls at the end
// Blob:  anything else, implemented here as a slice of bytes.
type Savedata struct {
	Forms   map[int]*Form
	Strings map[int]*String_chunk
	Blobs   map[int]Blob
}

// n is a modified index
func (sd *Savedata) Chunk(n int) Chunk {
	// Ugh... some kind of polymprphism might help here
	if sd.Forms[n] != nil {
		return sd.Forms[n]
	}
	if sd.Blobs[n] != nil {
		return sd.Blobs[n]
	}
	if sd.Strings[n] != nil {
		return sd.Strings[n]
	}

	panic(fmt.Sprintf("what offset? (%v)", n))
}

func (sd *Savedata) Game() Game {
	// We're dealing with RF iff the Valhalla<->Gaea jump point was originally hidden.
	game := GT_PRIV
	if sd.Forms[OFFSET_SSSS] != nil{
		hidden := sd.Forms[OFFSET_SSSS].Get("ORIG").Data
		if hidden[len(hidden)-1] == 68 {
			game = GT_RF
		}
	}
	// TODO: it's possible that we are being called in a "show_missing -rf context" - ideally, we should
	// get the -rf value here.

	return game
}

func read_header(r io.Reader) (Header, error) {
	//Header format:
	//
	// bytes 0x00-0x03: File size
	// bytes 0x04-??: Offsets
	//   Offsets are locations of things in the save file.  It is odd to see these in a save file format - perhaps it is also a memory dump?
	//   Each offset is 4 bytes.  Technically, only the first 2 bytes are the location; the last 2 bytes are always 00E0.  Maybe it's some sort of thunk?
	//   The number of offsets varies.  The named 9 in the offset enum are always present, but there are 2 more for each non-plot mission
	//   This number can be determined by peeking where the MISSIONS offset points, or by caculating based on the first offset.
	out := Header{}

	size, err := readers.Read_int_le(r)
	if err != nil {
		return out, err
	}
	out.File_size = size

	for i := 0; i <= OFFSET_MISSIONS; i += 1 {
		offset, err := readers.Read_int16(r)
		if err != nil {
			return out, err
		}
		out.Offsets = append(out.Offsets, offset)
		readers.Advance(r, 2)
	}

	// Data starts where offsets end, so offset[0] indirectly tells us how many offsets there are.
	// The -1 is for the file size.
	missions := (out.Offsets[0]/4 - OFFSET_COUNT - 1) / 2

	// Expect 2 more offsets for each mission
	mission_offsets := []int{}
	for i := 0; i < 2*missions; i += 1 {
		offset, err := readers.Read_int16(r)
		if err != nil {
			return out, err
		}
		mission_offsets = append(mission_offsets, offset)
		readers.Advance(r, 2)
	}

	for i := OFFSET_MISSIONS + 1; i < OFFSET_COUNT; i += 1 {
		offset, err := readers.Read_int16(r)
		if err != nil {
			return out, err
		}
		out.Offsets = append(out.Offsets, offset)
		readers.Advance(r, 2)
	}

	out.Offsets = append(out.Offsets, mission_offsets...)

	// TODO: advance to offsets[0]?

	return out, nil
}

// Read_savedata reads savedata (presumably, from a Privateer/RF savefile)
func Read_savedata(r io.ReadSeeker) (*Savedata, error) {
	header, err := read_header(r)
	if err != nil {
		return nil, err
	}
	out := Savedata{
		Forms:   map[int]*Form{},
		Strings: map[int]*String_chunk{},
		Blobs:   map[int]Blob{},
	}

	for true_index, _ := range header.Offsets {
		i := Modify_index(true_index, header.Missions())
		chunk_length := header.Offset_end(i) - header.Offsets[i]

		// An attempt was made to use only io.Reader for file reading.
		// It failed because mission forms sometimes lie about their lengths in a manner which claims the
		// first byte of the next chunk.  This means we can not rely on being at the start of chunk n+1 after reading chunk n.
		// This bullshit could in principle be caught and worked around, but it would massively complicate form reading.
		r.Seek(int64(header.Offsets[i]), io.SeekStart)
		switch header.Chunk_type(i) {
		case CT_FORM:
			f, err := Read_form(r)
			if err != nil {
				return nil, fmt.Errorf("Failed to load form at offset %v - %v", i, err)
			}
			out.Forms[i] = f

		case CT_STRING:
			str, n, err := readers.Read_string(r)
			if err != nil {
				return nil, errors.New("Failed to read string")
			}

			st := String_chunk{str, chunk_length}
			out.Strings[i] = &st
			// Variable-length string in fixed-length chunk
			readers.Advance(r, chunk_length-n)

		case CT_BLOB:
			blob, err := readers.Read_fixed(r, chunk_length)
			if err != nil {
				return nil, errors.New("Failed to read blob")
			}
			out.Blobs[i] = blob
		}
	}

	return &out, nil
}

func (sd *Savedata) Write(out io.Writer) {
	chunk_count := len(sd.Forms) + len(sd.Strings) + len(sd.Blobs)
	missions := (chunk_count - OFFSET_COUNT) / 2
	file_length := 4 * (1 + chunk_count) //header length
	for c := range chunk_count {
		file_length += sd.Chunk(c).Chunk_length()
	}

	// Header
	writers.Write_uint32_le(out, file_length)
	chunk_location := 4 * (1 + chunk_count)
	for c := range chunk_count {
		writers.Write_uint16_le(out, chunk_location)
		out.Write([]byte{0x00, 0xE0})
		chunk_location += sd.Chunk(Modify_index(c, missions)).Chunk_length()
	}

	//Body
	for c := range chunk_count {
		sd.Chunk(Modify_index(c, missions)).Write(out)
	}
}

type Record struct {
	Name   string
	Data   []byte
	Footer []byte
}

func (r *Record) Needs_footer() bool {
	return len(r.Data)%2 == 1
}

type Chunk interface {
	// Chunk length is the expected number of bytes needed to store the chunk in a file
	Chunk_length() int
	// Write writes the chunk, and returns bytes written (and a possible error)
	Write(io.Writer) (int, error)
}

type Form struct {
	Name     string
	Records  []*Record
	Subforms []*Form
	Tables   []*Table
	Footer   []byte
}

func (f *Form) Get(what ...string) *Record {
	for _, w := range what[:len(what)-1] {
		found := false
		for _, subform := range f.Subforms {
			if strings.HasSuffix(subform.Name, w) {
				f = subform
				//fmt.Println("Subform", f.Name)
				found = true
				break
			}
		}
		if !found {
			//fmt.Println("Failed to find Subform", w)
			return nil
		}
	}

	for i, rec := range f.Records {
		if strings.HasSuffix(rec.Name, what[len(what)-1]) {
			return f.Records[i]
		}
	}

	return nil
}

func (f *Form) Add_record(what ...string) *Record {
	for _, w := range what[:len(what)-1] {
		found := false
		for _, subform := range f.Subforms {
			if strings.HasSuffix(subform.Name, w) {
				f = subform //member functions are smoke and mirrors
				found = true
				break
			}
		}
		if !found {
			//fmt.Println("Failed to find Subform", w)
			return nil
		}
	}

	//fmt.Println("Name is",  what[len(what)-1])
	f.Records = append(f.Records, &Record{Name: what[len(what)-1], Data: []byte{}, Footer: nil})
	return f.Records[len(f.Records)-1]
}

func (f *Form) Get_subform(w string) *Form {
	//TODO: allow multiple args
	for _, subform := range f.Subforms {
		if strings.HasSuffix(subform.Name, w) {
			return f
		}
	}
	return nil
}

func (f *Form) Chunk_length() int {
	total := 12 //("FORM"(4), length(4), name(4))
	for _, rec := range f.Records {
		if rec.Name == "FORM" {
			continue
		}
		total += (len(rec.Name) + 4 + len(rec.Data) + (len(rec.Data) % 2)) //(name(4), length(4), data(whatever), footer(0 or 1))
	}
	for _, sf := range f.Subforms {
		total += sf.Chunk_length()
	}

	return total
}

func read_record(r io.Reader) (*Record, int, error) {
	bytes_read := 0

	record_name_buf, err := readers.Read_fixed(r, 4)
	if err != nil {
		return nil, 4, err
	}
	bytes_read += 4

	length, err := readers.Read_int_be(r)
	bytes_read += 4

	record_bytes, err := readers.Read_fixed(r, length)
	bytes_read += length

	record := &Record{string(record_name_buf), record_bytes, nil}
	if length%2 == 1 {
		record.Footer, _ = readers.Read_fixed(r, 1)
		bytes_read += 1
	}

	return record, bytes_read, nil
}

// Read_form reads a (almost IFF format) form
func Read_form(r io.Reader) (*Form, error) {
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

	_, err := readers.Read_fixed_string("FORM", r)
	if err != nil {
		return nil, err
	}

	length, err := readers.Read_int_be(r)
	if err != nil {
		return nil, err
	}

	return read_form_inner(r, length)
}

func read_form_inner(r io.Reader, length int) (*Form, error) {
	name_buf, err := readers.Read_fixed(r, 4)
	if err != nil {
		return nil, err
	}
	bytes_read := 4
	out := Form{Name: string(name_buf)}

	// records
	for bytes_read <= length-8 { // Minimum record size is 8

		record, read, err := read_record(r)
		if err != nil {
			fmt.Println("Unable to read record")
			//fmt.Println(fmt.Sprintf("Ignoring %v footer at %v: %v", out.Name, *cur, bytes[*cur:form_end]))
			break
		}
		bytes_read += read

		if record.Name == "TABL" {
			// TABL is a non-standard "record type" which doesn't even fit the definition of a record
			table := Table{}
			for i := 0; i < len(record.Data); i += 4 {
				offset, _ := readers.Read_int_le(bytes.NewReader(record.Data[i : i+4])) //todo: fewer readers
				table.Offsets = append(table.Offsets, offset)
			}

			// The offsets are offsets into the *form*, starting at the very beginning.  Because of fucking course they are.
			for _, offset := range table.Offsets {
				foffset := offset - 8 // bytes_read counts from after the "FORM" and form length (4 bytes each).  So fudge the offset.
				if bytes_read < foffset {
					readers.Advance(r, foffset-bytes_read)
					bytes_read = foffset
				}
				if bytes_read > foffset {
					panic("messed up table!")
				}

				expected_length, _ := readers.Read_int_le(r)
				bytes_read += 4

				table_record, actual_length, err := read_record(r)
				if err != nil {
					panic("Messed-up table!")
				}
				bytes_read += actual_length
				if actual_length != expected_length {
					panic(fmt.Sprintf("Messed-up table! read %v, should have read %v", actual_length, expected_length))
				}

				table.Records = append(table.Records, table_record)
			}
			out.Tables = append(out.Tables, &table)
		} else {
			// Normal Record
			//fmt.Println("Adding", record_name, "to", out.name)
			out.Records = append(out.Records, record)
		}

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
	out.Footer, _ = readers.Read_fixed(r, length-bytes_read)

	return &out, nil
}

func (form *Form) Write(out io.Writer) (int, error) {
	writers.Write_string_padded(out, "FORM", 4)
	writers.Write_uint32_be(out, form.Chunk_length()-8)
	writers.Write_string_padded(out, form.Name, 4)

	sub := 0
	for r, record := range form.Records {
		if record.Name == "FORM" {
			form.Subforms[sub].Write(out)
			sub += 1
			continue
		}
		writers.Write_string_padded(out, record.Name, len(record.Name))
		writers.Write_uint32_be(out, len(record.Data))
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

	// TODO: catch errors (and size)
	return form.Chunk_length(), nil
}

func (f *Form) String() string {
	out := f.Name + "\n"
	for _, record := range f.Records {
		if record.Name != "FORM" {
			out += fmt.Sprintf("%v\n", record)
		}

	}
	for _, subform := range f.Subforms {
		out += subform.String()
	}
	for _, table := range f.Tables {
		out += table.String()
	}
	return out
}

type Table struct {
	Offsets []int
	Records []*Record
}

func (t *Table) String() string {
	out := "TABL\n"
	for _, record := range t.Records {
		out += fmt.Sprintf("%v\n", record)
	}
	return out
}

// Blob ([B]inary, possibly [L]arge, [OB]ject) is a []byte with just enough stuff added to implement the Chunk interface
type Blob []byte

func (b Blob) Chunk_length() int {
	return len(b)
}

func (b Blob) Write(w io.Writer) (int, error) {
	return w.Write(b)
}

// String_chunk holds a string and a chunk length
// (since the string is always stored with a null terminator, max string length is 1 less than chunk length)
type String_chunk struct {
	Value  string
	Length int // chunk length not string length
}

func (sc *String_chunk) Chunk_length() int {
	return sc.Length
}

func (sc *String_chunk) Write(w io.Writer) (int, error) {
	return writers.Write_string_padded(w, sc.Value, sc.Length)
}

func (sc *String_chunk) String() string {
	return fmt.Sprintf("%v (max length %v)", sc.Value, sc.Length-1)
}
