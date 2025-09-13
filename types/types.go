package types

import (
	"fmt"
	"strings"
)

type Game int

const (
	GT_NONE Game = iota
	GT_PRIV
	GT_RF
)

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
	CT_BLOB ChunkType = iota
	CT_FORM
	CT_STRING
)

type Header struct {
	File_size int
	Offsets   []int
}

func (h *Header) Modify_index(i int) int {
	// TODO: unduplicate?
	missions := (h.Offsets[0]/4 - OFFSET_COUNT - 1) / 2

	if i > OFFSET_MISSIONS && i <= OFFSET_MISSIONS+2*missions {
		i = i - OFFSET_MISSIONS + OFFSET_COUNT - 1
	} else {
		if i > OFFSET_MISSIONS+2*missions {
			i -= 2 * missions
		}
	}

	return i
}

// i: true chunk index.
func (h *Header) Chunk_type(i int) ChunkType {
	// TODO: unduplicate?
	missions := (h.Offsets[0]/4 - OFFSET_COUNT - 1) / 2

	if i > OFFSET_MISSIONS && i <= OFFSET_MISSIONS+2*missions {
		// This is mission-related
		if i%2 == 0 {
			return CT_FORM
		}
		return CT_STRING
	}
	if i > OFFSET_MISSIONS+2*missions {
		i -= 2 * missions
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
	missions := (h.Offsets[0]/4 - OFFSET_COUNT - 1) / 2

	if o == OFFSET_MISSIONS && len(h.Offsets) > OFFSET_COUNT {
		// Jump to mission offsets
		return h.Offsets[OFFSET_MISSION_BASE]
	}
	if missions > 0 && o == len(h.Offsets)-1 {
		// At the end of mission offsets, jump back
		return h.Offsets[OFFSET_PLAY]
	}
	if o == OFFSET_CALLSIGN {
		// Annoying special case: since this is the last offset, we can't look at the next one to see where it ends.
		// So we just hard-code its known length.  Ugh!!!
		return h.Offsets[o] + 15
	}

	return h.Offsets[o+1]
}

// Savedata stores mostly-parsed data from a savefile
// Data at each offset fits into one of 3 categories:
// Form: see the form class
// String: null-terminated string, usually inside a fixed-length space with extra nulls at the end
// Blob:  anything else, referred to here as a slice of bytes.
type Savedata struct {
	Forms   map[int]*Form
	Strings map[int]string
	Blobs   map[int][]byte
}

// n is a modified index
func (sd *Savedata) Chunk_length(n int) int {
	// Ugh... some kind of polymprphism might help here
	if sd.Forms[n] != nil {
		return sd.Forms[n].Real_size()
	}
	if n == OFFSET_NAME {
		return 18
	}
	if n == OFFSET_CALLSIGN {
		return 15
	}
	if n >= OFFSET_MISSION_BASE && (n-OFFSET_MISSION_BASE)%2 == 0 {
		return 8
	}
	if sd.Blobs[n] != nil {
		return len(sd.Blobs[n])
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

type Record struct {
	Name   string
	Data   []byte
	Footer []byte
}

func (r *Record) Needs_footer() bool {
	return len(r.Data)%2 == 1
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
			// Do not return a copy, caller may be getting to edit
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

func (f *Form) Real_size() int {
	total := 12 //("FORM"(4), length(4), name(4))
	for _, rec := range f.Records {
		if rec.Name == "FORM" {
			continue
		}
		total += (4 + len(rec.Name) + len(rec.Data) + (len(rec.Data) % 2)) //(name(4), length(4) +data(whatever) + footer)
	}
	for _, sf := range f.Subforms {
		total += sf.Real_size()
	}

	return total
}
