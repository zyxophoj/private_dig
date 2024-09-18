package types

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
	Name  string
	Data  []byte
	Forms []Form
}

type Form struct {
	Name    string
	Length  int
	Records []Record
	Footer  []byte
}
