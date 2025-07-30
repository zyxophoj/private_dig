package types

import (
	//"fmt"
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

	OFFSET_COUNT // This only counts the always-present offsets
	OFFSET_MISSION_BASE = OFFSET_COUNT // Used as an internal index for the first non-plot mission
)

func Offset_name(o int) string {
	return []string{"ShipLocGuilds", "Plot", "Mission count", "Score", "WTF", "Hidden Jumps", "Equipment", "Name", "Callsign",
	                "Mission 1 Name",  "Mission 1 Data", "Mission 2 Name",  "Mission 2 Data", "Mission 3 Name",  "Mission 3 Data"}[o]
}

type Header struct {
	File_size       int
	Offsets         []int
	Footer          []byte
}

// Offset_end returns the index of the byte one after the end of the offset with the given offset ID.
// This will normally be h.Offsets[o+1], but enum order not matching file order (due to moving missions to the end of the list) complicates things.
func (h *Header) Offset_end(o int) int {
	if o == OFFSET_MISSIONS && len(h.Offsets) > OFFSET_COUNT {
		// Jump to mission offsets
		return h.Offsets[OFFSET_MISSION_BASE]
	}
	if o == len(h.Offsets)-1 {
		// Jump back to always-present offsets
		return h.Offsets[OFFSET_PLAY]
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

type Record struct {
	Name string
	Data []byte
}

type Form struct {
	Name     string
	Length   int
	Records  []Record
	Footer   []byte
	Subforms []Form
}

func (f *Form) Get(what ...string) *Record {
	for _, w := range what[:len(what)-1] {
		found := false
		for _, subform := range f.Subforms {
			if strings.HasSuffix(subform.Name, w) {
				f = &subform
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

	for _, rec := range f.Records {
		if strings.HasSuffix(rec.Name, what[len(what)-1]) {
			return &rec
		}
	}

	return nil
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
