package types

import (
	"fmt"
	"io"
	"privdump/writers"
	"strings"
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
	hidden := sd.Forms[OFFSET_SSSS].Get("ORIG").Data
	if hidden[len(hidden)-1] == 68 {
		game = GT_RF
	}

	return game
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
	Length   int
	Records  []*Record
	Subforms []*Form
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
		total += (4 + len(rec.Name) + len(rec.Data) + (len(rec.Data) % 2)) //(name(4), length(4) +data(whatever) + footer)
	}
	for _, sf := range f.Subforms {
		total += sf.Chunk_length()
	}

	return total
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
	string
	length int // chunk length not string length
}

func Make_String_chunk(str string, l int) String_chunk {
	return String_chunk{str, l}
}

func (sc String_chunk) Chunk_length() int {
	return sc.length
}

func (sc String_chunk) Write(w io.Writer) (int, error) {
	return writers.Write_string_padded(w, sc.string, sc.length)
}

func (sc String_chunk) Get() string {
	return sc.string
}

func (sc String_chunk) Set(to string) error {
	if len(to)+1 > sc.length{
		return fmt.Errorf("String [%v] is too long for a chunk of length %v", to, sc.length)
	}
	sc.string = to
	return nil
}


