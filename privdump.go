package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"slices"
	"strings"

	"privdump/tables"
)

const (
	OFFSET_SHIP = iota  // Ship type, location, guild membership
	OFFSET_PLOT  //Plot status
	OFFSET_MISSIONS
	OFFSET_PLAY  // player kill count + reputation
	OFFSET_WTF
	OFFSET_SSSS  // Hidden jump points?
	OFFSET_REAL  // ship equipment
	OFFSET_NAME
	OFFSET_CALLSIGN

	OFFSET_COUNT
)

func offset_name(o int) string {
	return []string{"ShipLocGuilds", "Plot", "Mission count", "Score", "WTF", "Hidden Jumps", "Equipment", "Name", "Callsign"}[o]
}

type Header struct {
	file_size       int
	offsets         []int
	mission_offsets []int
	footer          []byte
}

type Record struct {
	name  string
	data  []byte
	forms []Form
}

type Form struct {
	name    string
	length  int
	records []Record
	footer  []byte
}

func read_string(bytes []byte, cur *int) (string, int, error) {
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

func read_fixed_string(target string, bytes []byte, cur *int) (int, error) {
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

func read_uint8(bytes []byte, cur *int) uint8 {
	out := bytes[*cur]
	*cur += 1
	return out
}

func read_int(bytes []byte, cur *int) int {
	// big-endian
	out := uint(0)
	for _ = range 4 {
		out = out << 8
		out = out + uint(bytes[*cur])
		*cur += 1
	}

	return int(out)
}

func read_int_le(bytes []byte, cur *int) int {
	// little-endian
	out := uint(0)
	for i := range 4 {
		out = out + uint(bytes[*cur])<<(8*i)
		*cur += 1
	}

	return int(out)
}

func read_int16(bytes []byte, cur *int) int {
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

func read_header(in []byte) Header {
	//Header format:
	//
	// bytes 0x00-0x03: File size
	// bytes 0x04-??: Offsets
	//   Offsets are locations of things in the save file.  It is odd to see these in a save file format - perhaps it is also a memory dump?
	//   Each offset is 4 bytes.  Technically, only the first 2 bytes are the location; the last 2 bytes are always 00E0.  Maybe it's some sort of thunk?
	//   The number of offsets varies.  The named 9 in the offset enum are always present, but there are 2 more for each non-plot mission
	//   This number can be determined by peeking where the MISSIONS offset points.  (Or by caculating based on the first offset?  Are we sure there's never a footer??)
	// bytes ??-?? : footer
	//   That which lies between the offset block and the first offset.
	out := Header{}

	cur := 0
	out.file_size = read_int16(in, &cur)
	cur += 2
	for i := 0; i <= OFFSET_MISSIONS; i += 1 {
		out.offsets = append(out.offsets, read_int16(in, &cur))
		cur += 2
	}

	// Peek at the data  (TODO: or use offset[0])
	cur2 := out.offsets[OFFSET_MISSIONS]
	missions := read_int16(in, &cur2)

	// Expect 2 more offsets for each missions
	for i := 0; i < 2*missions; i += 1 {
		out.mission_offsets = append(out.mission_offsets, read_int16(in, &cur))
		cur += 2
	}

	for i := OFFSET_MISSIONS + 1; i < OFFSET_COUNT; i += 1 {
		out.offsets = append(out.offsets, read_int16(in, &cur))
		cur += 2
	}

	out.footer = in[cur:out.offsets[0]]
	cur = out.offsets[0]

	return out
}

func read_form(bytes []byte, cur *int) (Form, error) {
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

	out := Form{}

	_, err := read_fixed_string("FORM", bytes, cur)
	if err != nil {
		return out, err
	}

	out.length = read_int(bytes, cur)
	form_start := *cur
	form_end := form_start + out.length

	out.name = string(bytes[*cur : *cur+4])
	*cur += 4

	for *cur <= form_end-8 { // Minimum record size is 8
		record_name, _, err := read_string(bytes[:form_end], cur)
		if err != nil {
			fmt.Println("Unable to read record")
			fmt.Println(fmt.Sprintf("Ignoring %v footer at %v: %v", out.name, *cur, bytes[*cur:form_end]))
			*cur = form_end
			break
		}
		length := read_int(bytes, cur)
		record_start := *cur
		//fmt.Println(fmt.Sprintf("Record %v  %v->%v", record_name, *cur, *cur+length))

		record := Record{record_name, bytes[*cur : *cur+length], nil}

		if strings.HasSuffix(record_name, "FORM") {
			*cur -= 8 // EVIL HACK!! go back and re-parse this record as a form.
			// Subforms!!!
			for *cur < record_start+length {
				form, err := read_form(bytes, cur)
				if err != nil {
					//fmt.Println(bytes[*cur:record_start+length])
					//*cur = record_start+length
					break
				}

				//fmt.Println("Adding", form.name, "to", record.name)
				record.forms = append(record.forms, form)
			}

		} else {
			*cur += length
		}

		//fmt.Println("Adding", record_name, "to", out.name)
		out.records = append(out.records, record)
	}

	out.footer = bytes[*cur:form_end]
	*cur = form_end

	return out, nil
}

func parse_header(header Header, bytes []byte) []string {
	out := []string{}

	out = append(out, fmt.Sprintf("1-4: File size (%v)", header.file_size))
	cur := 0

	for o := 0; o < len(header.offsets); o += 1 {
		cur = header.offsets[o]
		out = append(out, "")
		out = append(out, fmt.Sprintf("%v-%v: %v offset (x%x)", 5+4*o, 8+4*o, offset_name(o), cur))
		switch o {
		case OFFSET_SHIP:
			ships := map[uint8]string{
				0: "Tarsus",
				1: "Orion",
				2: "Centurion",
				3: "Galaxy",
			}
			out = append(out, fmt.Sprintf("   %v: Ship: %v", cur, safe_lookup(ships, bytes[cur])))
			cur += 2

			loc := read_uint8(bytes, &cur)
			out = append(out, fmt.Sprintf("   %v: Location: %v", cur-1, safe_lookup(tables.Locations, loc)))

			out = append(out, fmt.Sprintf("   %v-%v: Unknown - %v", cur, cur+1, bytes[cur:cur+2]))
			cur += 2

			guild_status := map[uint8]string{
				0: "Nonmember",
				1: "Member",
			}
			out = append(out, fmt.Sprintf("   %v: Merchants' Guild: %s", cur, safe_lookup(guild_status, bytes[cur])))
			cur += 1
			out = append(out, fmt.Sprintf("   %v: Mercenaries' Guild: %s", cur, safe_lookup(guild_status, bytes[cur])))
			cur += 1

		case OFFSET_PLOT:
			status, _, _ := read_string(bytes, &cur)
			if status == "" {
				out = append(out, fmt.Sprintf("   (Plot has not been started?)"))
			} else if status == "FFFFFFFF" {
				out = append(out, fmt.Sprintf("   (Plot failed!)"))
			} else {
				// This section begins with something like "s4m2" indicating series and mission number
				series := map[string]string{
					"s0": "Sandoval",
					"s1": "Tayla",
					"s2": "Roman Lynch", //and Miggs!
					"s3": "Oxford",
					"s4": "Lynn Murphy",
					"s5": "Dr Monkhouse",
					"s6": "Taryn Cross",
					"s7": "Final",
				}
				out = append(out, fmt.Sprintf("   Series: %v, Mission %v", safe_lookup(series, status[0:2]), status[3:4]))
			}

			// add 8+1 because this thing is long enough to accommodate the failing "FFFFFFFF" string.
			// There remains one poorly understood byte.
			final := bytes[header.offsets[o]+8+1 : header.offsets[o+1]] // This looks like a bitfield
			mstatus := map[uint8]string{
				160: "Accepted",
				226: "Failed",
				255: "Complete",
			}
			out = append(out, fmt.Sprintf("   Status: %v", safe_lookup(mstatus, final[0])))
			// This byte can't tell the difference between "You haven't talked to someone yet", and "you talked, rejected, but they'll still be here if you change your mind"
			// That info is in the WTF section... somewhere.

		case OFFSET_MISSIONS:
			// 2-bytes, loks like just the mission count
			missions := read_int16(bytes, &cur)
			out = append(out, fmt.Sprintf("   Non-plot missions: %v", missions))
		case OFFSET_WTF:
			// Basically 0% understood right now
			// Probably has something to do with fixer status
			out = append(out, fmt.Sprintf("  %v", bytes[cur:header.offsets[o+1]]))
		case OFFSET_PLAY, OFFSET_SSSS, OFFSET_REAL:
			// It's just a form...
			form, err := read_form(bytes, &cur)
			if err != nil {
				out = append(out, fmt.Sprintf("Bad form!  error:%v", err))
				break
			}
			out = append(out, parse_form("", form)...)

		case OFFSET_NAME, OFFSET_CALLSIGN:
			s, _, _ := read_string(bytes, &cur)
			out = append(out, fmt.Sprintf("   %v: %v", offset_name(o), s))
		}
	}

	// OK, now do missions
	for m := 0; m < len(header.mission_offsets)/2; m += 1 {
		cur = header.mission_offsets[2*m]
		name, _, _ := read_string(bytes, &cur)
		out = append(out, fmt.Sprintf("[%v] Mission %v name: %v", header.mission_offsets[2*m], m, name))

		cur = header.mission_offsets[2*m+1]
		// Another form!
		form, err := read_form(bytes, &cur)
		if err != nil {
			out = append(out, fmt.Sprintf("Bad form!  error:%v", err))
			break
		}
		out = append(out, parse_form("", form)...)

	}

	return out
}

func parse_form(prefix string, form Form) []string {
	out := []string{}
	for _, r := range form.records {
		out = append(out, parse_record(prefix+form.name+"-", r))
	}
	if len(form.footer) > 0 {
		out = append(out, fmt.Sprintf("Ignored footer in form %v, %v", form.name, form.footer))
	}

	return out
}

func parse_record(prefix string, record Record) string {
	out := ""

	factions := []string{"Merchants", "Hunters", "Confeds", "Kilrathi", "Militia", "Pirates", "Drone", "", "Retros"}

	// Record format depends on record name
	// record name itself is rather odd, as there seems to be alternate names for the same thing, varying only by doubled first letter
	// (I suspect this is some kind of off-by-one error in writing)
	record_name2 := record.name
	if len(record_name2) == 5 && record_name2[0] == record_name2[1] {
		record_name2 = record_name2[1:]
	}

	if record_name2 != "FORM" {
		out += "Record: " + prefix + record.name + fmt.Sprintf("%v\n", record.data)
	}

	switch record_name2 {
	case "SCOR":
		// Reputation depends only on ship classes killed, which results in strange effects for special enemies
		// e.g. killing Black Rhombus will make pirates like you more, since it is treated like any other Galaxy.
		// There are 3 different Talon types in order to make this work.
		status := func(f int) string {
			if f > 25 {
				return "friendly"
			}
			if f < -25 {
				return "hostile"
			}

			return "neutral"
		}

		out += "Reputation:\n"
		for i := range factions {
			cur := 2 * i
			v := read_int16(record.data, &cur)
			if factions[i] != "" {
				out += fmt.Sprintf("%s: %v (%s)\n", factions[i], v, status(v))
			}
		}

	case "KILL":
		// Kill count can be displayed on the in-game computer and it behaves correctly for special enemies
		// (e.g. Black Rhombus is a pirate not a merchant, Mordecai Jones is a retro not a hunter)
		out += "Kills:\n"
		for i := range factions {
			cur := 2 * i
			v := read_int16(record.data, &cur)
			if factions[i] != "" || v > 0 {
				out += fmt.Sprintf("%s: %v\n", factions[i], v)
			}
		}

	case "ORIG":
		// This seems to be constant no matter what we do.
		expected := []byte{0x33, 0x3B, 0x3B, 0x3D, 0x3D, 0x3C, 0x3C, 0x3E}
		if slices.Equal(record.data, expected) {
			out += "Normal ORIG\n"
		} else {
			out += fmt.Sprintf("Unusual ORIG!!! Expected %v; found %v\n", expected, record.data)
		}

		//case "SECT"
		//TODO: understanding this one is very important, because it seems to be where the unexplored jump point un-hiding happens.

	case "GUNS":
		out += "Guns:\n"
		// This is a list of 4-byte entries - {gun_type, location, damage?, ??}
		// If the gun is in a turret, this gets a little wonky, becuase mounts 5-7 are
		// "first turret", which may be top or rear depending on ship.
		guns := map[int]string{
			5: "Laser",
			3: "Mass Driver",
			1: "Meson Blaster",
			0: "Neutron gun",
			4: "Particle Cannon",
			7: "Tachyon Cannon",
			2: "Ionic Pulse Cannon",
			6: "Plasma Gun",
			8: "Steltek Gun",

			// This one lacks an official name, but the Steltek say they attach
			// a power booster, so let's go with that.
			9: "Boosted Steltek Gun",
		}
		mounts := map[int]string{
			1: "Left outer",
			2: "Left",
			3: "Right",
			4: "Right outer",
			5: "Rear/Top 2",
			//6: tractor beam slot
			7: "Rear/Top 1",
			8: "Bottom 2",
			//9: tractor beam slot
			10: "Bottom 1",
		}
		for i := range len(record.data) / 4 {
			gun := int(record.data[i*4])
			mount := int(record.data[i*4+1])
			// TODO: Next byte indicates damage, but how?

			out += fmt.Sprintf("%v: %v\n", safe_lookup(mounts, mount), safe_lookup(guns, gun))
		}

	case "LNCH":
		out += "Launchers:\n"
		launchers := map[int]string{
			50: "Missile Launcher",
			51: "Torpedo Launcher",
			52: "Tractor Beam",
		}
		mounts := map[int]string{
			0: "Centre",
			1: "Left",
			2: "Left",
			3: "Right",
			4: "Right",

			6: "Turret 1",
			9: "Turret 2",
		}
		for i := range len(record.data) / 4 {
			launcher := int(record.data[i*4])
			mount := int(record.data[i*4+1])
			out += fmt.Sprintf("%v: %v\n", safe_lookup(mounts, mount), safe_lookup(launchers, launcher))
		}

	case "MISL":
		out += "Missiles:\n"
		missiles := map[uint8]string{
			1: "Torpedo",
			4: "Dumbfire",
			2: "Heat Seeker",
			5: "Image Rec",
			3: "Friend or Foe",
		}
		for i := range len(record.data) / 3 {
			msl_type := record.data[i*3]
			count := record.data[i*3+1]
			out += fmt.Sprintf("%v: %v\n", safe_lookup(missiles, msl_type), count)
		}

	case "TRRT":
		out += "Turrets:\n"
		// This only counts turrets, not what you have in them.
		turrets := map[uint8]string{
			1: "Rear",
			2: "Top",
			3: "Bottom",
		}
		for i := range len(record.data) {
			out += fmt.Sprintf("%v\n", safe_lookup(turrets, record.data[i]))
		}

	case "NAVQ":
		// This one's a bitfield that tracks which of the 4 quadrant maps we have.
		maps := map[uint8]string{
			1: "Humboldt",
			2: "Farris",
			4: "Potter",
			8: "Clarke",
		}

		out += "Maps:\n"
		// Short description for the overwhelmingly most common case
		if record.data[0] == 15 {
			out += "All\n"
			break
		}
		for k, v := range maps {
			if k&record.data[0] != 0 {
				out += v + "\n"
			}
		}

	case "AFTB":
		// A 0-length record.  Either you have afterburners or you don't.
		out += "Afterburners:\n"
		out += "(present)\n"

	case "ECMS":
		// A 1-length record.
		// The manual states that the 3 levels of ECM are 25%, 50% and 75% effective.
		// It looks like the ID here is doing double duty as effectiveness%.
		out += "ECM:\n"
		out += fmt.Sprintf("%v%% effective\n", record.data[0])

	case "CRGI":
		out += "Cargo-info?:\n"
		// What do credits and cargo expansions have in common?  I'd like to know what they were thinking on this one.
		cur := 0
		out += fmt.Sprintf("Credits: %v\n", read_int_le(record.data, &cur))
		boolmap := map[bool]string{true: "Yes", false: "No"}
		out += fmt.Sprintf("Capacity: %vT, Secret compartment: %v; expanded: %v\n", record.data[4], boolmap[record.data[6] != 0], boolmap[record.data[7] != 0])

	case "REPR":
		out += "Repair Droid:\n"
		// There doesn't seem to be any variation here
		// TODO: check RF's super repair droid
		expected := []byte{0x90, 1, 0, 0}
		if slices.Equal(record.data, expected) {
			out += "Normal\n"
		} else {
			out += fmt.Sprintf("Unusual Repair Droid!!! Expoected %v; found %v\n", expected, record.data)
		}

	case "ARMR":
		// 16 bytes, which looks like 8 16-bit ints.
		// First 4 entries are fully-repaired values - which also seem to be doing double duty as armor type.
		// These are always the same, so we only bother with the first.
		cur := 0
		out += "Armour:\n"
		names := map[int]string{
			0:   "(none)",
			250: "Plasteel",
			500: "Tungsten",
		}
		armor_type := read_int16(record.data, &cur)
		out += fmt.Sprintf("Armour type:%v\n", safe_lookup(names, armor_type))
		cur += 6

		// Avoid calculating percentages when they are 0/0 !!
		if armor_type == 0 {
			out += "None\n"
			cur += 8
			break
		}

		// Next 4 are actual armor values
		// We can give percentages here, but can't usefully give absolute values, because
		// this section doesn't seem to care about what ship we're flying - so Tarsus armour
		// looks exactly the same as Orion armour despite the manual (and practical experience
		// of just how long it takes to die in a crippled Orion) telling us Orion armour is
		// about 5 times as thick.
		for _, f := range []string{"Front", "Left", "Right", "Back"} {
			out += fmt.Sprintf("%v: %v%%\n", f, read_int16(record.data, &cur)*100/armor_type)
		}

	case "INFO":
		cur := 0

		if strings.HasSuffix(prefix, "JDRV-") {
			out += "Jump drive info\n"
			out += fmt.Sprintf("Jumps: %v\n", read_int16(record.data, &cur))
			out += fmt.Sprintf("Capacity?: %v\n", read_int16(record.data, &cur))
			break
		}

		infotype, _, _ := read_string(record.data, &cur)
		out += "INFO type " + infotype + "\n"
		switch infotype {
		case "SHIELDS":
			out += fmt.Sprintf("Shields level %v\n", int(record.data[cur+1])-89) //WHY???
		case "ENERGY":
			d := record.data[len("ENERGY")+2 : len(record.data)]
			strd := ""
			for _, n := range d {
				strd += fmt.Sprintf("%v", n)
			}
			// Yes, really.  There is clearly some structure in here, but I can't make any sense out of it.
			levels := map[string]string{
				"1261":         "(None)",
				"124151":       "Level 1",
				"12314151":     "Level 2",
				"1231415162":   "Level 3",
				"122131415161": "Level 4",
				"122231415161": "Level 5",
			}
			out += fmt.Sprintf("Engine: %v\n", safe_lookup(levels, strd))
		default:
			out += fmt.Sprintf("Unknown info type: %v\n", infotype)

		}

	case "DATA":
		// TODO: is this really only cargo data?

		// Each 4-byte block is: cargo-type, quantity(2 bytes!), hiddenness
		// Note that if illegal cargo spills out into the non-secret area, it will have 2 entries here
		// Maximum cargo capacity is an upgraded Galaxy with secret compartment, totalling 245T,
		// so 2 bytes for quantity seems excessive, but you can edit yourself over 255T of stuff
		// by hitting that second byte.
		out += "Cargo data:\n"
		for cur := 0; cur < len(record.data); {
			cargo := read_uint8(record.data, &cur)
			quantity := read_int16(record.data, &cur)
			hidden := read_uint8(record.data, &cur)

			hiddenness := map[uint8]string{
				0: "",
				1: " (hidden)",
			}
			out += fmt.Sprintf("%v (%vT)%v\n", safe_lookup(tables.Cargo, cargo), quantity, safe_lookup(hiddenness, hidden))
		}

	case "TEXT":
		// Mission text that displays in the in-game computer.  It's just text.
		out += "\n" + string(record.data[1:]) + "\n"

	case "CARG":
		// Mission cargo - a 3-byte field...
		// Byte 0: destination
		// Byte 1: always 49 - this could be cargo type, since missions are always "mission cargo", even when the descriptions say they are not.
		// Byte 2: How many tons
		out += fmt.Sprintf("Deliver %vT of %v to %v\n", record.data[2], safe_lookup(tables.Cargo, record.data[1]), safe_lookup(tables.Locations, record.data[0]))

	case "PAYS":
		//Mission payment (4 bytes, although I've never seen a mission that needed that)
		cur := 0
		pays := read_int_le(record.data, &cur)
		out += fmt.Sprintf("%v credits\n", pays)

	case "FORM":
		// Do nothing!  Subforms are handled at the end of the functon.

	default:
		out += fmt.Sprintf("(don't know how to parse %v)\n", record.name)
	}

	for _, f := range record.forms {
		out += strings.Join(parse_form(prefix, f), "\n")
	}

	return out
}

func main() {

	basedir := "C:\\Program Files (x86)\\GOG Galaxy\\Games\\Wing Commander Privateer\\cloud_saves\\"

	filename := os.Args[1]
	full_filename := basedir + filename

	bytes, err := ioutil.ReadFile(full_filename)
	if err != nil {
		fmt.Println("Failed to load file", full_filename, "-", err)
		os.Exit(-1)
	}

	header := read_header(bytes)
	fmt.Println()
	for _, line := range parse_header(header, bytes) {
		fmt.Println(line)
	}
	fmt.Println()

}
