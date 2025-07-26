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

	OFFSET_COUNT
)

func Offset_name(o int) string {
	return []string{"ShipLocGuilds", "Plot", "Mission count", "Score", "WTF", "Hidden Jumps", "Equipment", "Name", "Callsign"}[o]
}

type Header struct {
	File_size       int
	Offsets         []int
	Mission_offsets []int
	Footer          []byte
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
