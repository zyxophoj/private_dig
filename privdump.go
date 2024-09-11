package main

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"slices"
	"strings"
)

type Record struct {
	name  string
	data  []byte
	forms []Form
}

type Form struct {
	name    string
	length  int
	records []Record
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

func read_form(bytes []byte, cur *int) (Form, error) {
	// Form format:
	//
	// 1 Identifier: A 4-byte capital-letter string which is (usually, but not always??), "FORM"
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
	// 3 Data: could be anything, but there is one very special case:  If the name is "FORM" then data is a sequence of forms.
	//
	// Again, length does not include the first 8 bytes

	out := Form{"", 0, []Record{}}

	_, err := read_fixed_string("FORM", bytes, cur)
	if err != nil {
		return out, err
	}

	out.length = read_int(bytes, cur)
	form_start := *cur
	form_end := form_start + out.length

	out.name = string(bytes[*cur : *cur+4])
	*cur += 4

	for *cur < form_end {
		record_name, _, err := read_string(bytes[:form_end], cur)
		if err != nil {
			fmt.Println("Unable to read record")
			fmt.Println(fmt.Sprintf("Ignoring %v footer: %v", out.name, bytes[*cur:form_end]))
			*cur = form_end
			break
		}
		length := read_int(bytes, cur)
		record_start := *cur
		//fmt.Println(fmt.Sprintf("Record %v  %v->%v", record_name, *cur, *cur+length))

		record := Record{record_name, bytes[*cur : *cur+length], nil}

		if strings.HasSuffix(record_name, "FORM") {
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

	return out, nil
}

func parse_header(header []byte) []string {
	out := []string{}

	// Header is a fixed-length 62-byte blob
	// We don't know what most of this does.

	// 00-01 file size
	cur := 0
	size := read_int16(header, &cur)
	out = append(out, fmt.Sprintf("00-01 Reported file size: %v bytes", size))

	//2-3  Always zeroes?  Technically part of file size?

	// 4-27 offsets...
	// Offsets are locations of things in the save file.  It is odd to see these in a save file format - perhaps it is also a memory dump?
	// Technically, only the first 2 bytes are the offset; the last 2 bytes are always 00E0.  Maybe it's some sort of thunk?
	// If this dumper ever becomes an editor then we need to care about these, but for the moment we just dump the values.

	//4-F: ????? looks like 3 offsets into the header itself.  They are not fixed - random missions can mess them up - and we should use them to read the rest of this file.
	cur = 0x10
	offset := read_int16(header, &cur)
	out = append(out, fmt.Sprintf("10-13 forms start offset: %v bytes", offset))

	// 10-27 offsets
	cur = 0x14
	offset = read_int16(header, &cur)
	out = append(out, fmt.Sprintf("14-17 PLAY end offset: %v bytes", offset))

	cur = 0x18
	offset = read_int16(header, &cur)
	out = append(out, fmt.Sprintf("18-1B SSSS form offset: %v bytes", offset))

	cur = 0x1C
	offset = read_int16(header, &cur)
	out = append(out, fmt.Sprintf("1C-1F REAL form offset: %v bytes", offset))

	cur = 0x20
	offset = read_int16(header, &cur)
	out = append(out, fmt.Sprintf("20-23 name offset: %v bytes", offset))

	cur = 0x24
	offset = read_int16(header, &cur)
	out = append(out, fmt.Sprintf("24-27 callsign offset: %v bytes", offset))

	// 28 : ship
	ships := map[uint8]string{
		0: "Tarsus",
		1: "Orion",
		2: "Centurion",
		3: "Galaxy",
	}
	ship, ok := ships[header[0x28]]
	if !ok {
		ship = fmt.Sprintf("Unknown - %v", header[0x28])
	}
	out = append(out, fmt.Sprintf("28-28 Ship: %v", ship))

	// 29 - always zero?  Technically part of ship?

	// 2A-2B : Location?
	cur = 0x2A
	loc := read_int16(header, &cur)
	out = append(out, fmt.Sprintf("2A-2B Location: %v", loc))

	// 2C??

	// 2D-2E
	guild_status := func(b byte) string {
		switch b {
		case 0:
			return "Nonmember"
		case 1:
			return "Member"
		}

		return fmt.Sprintf("Unexpected: %v", b)
	}
	out = append(out, fmt.Sprintf("2D-2D Merchants' Guild: %s", guild_status(header[0x2D])))
	out = append(out, fmt.Sprintf("2E-2E Mercenaries' Guild: %s", guild_status(header[0x2E])))

	//2F-3C ????  14 bytes, probably something to do with plot progression.

	return out
}

func parse_record(prefix string, record Record) string {
	out := ""

	factions := []string{"Merchants", "Hunters", "Confeds", "Kilrathi", "Militia", "Pirates", "Drone", "", "Retros"}

	if record.name != "FORM" {
		out += "Record: " + prefix + record.name + fmt.Sprintf("%v\n", record.data)
	}

	// Record format depends on record name
	switch record.name {
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
			gun_type := int(record.data[i*4])
			mount := int(record.data[i*4+1])
			// TODO: Next byte indicates damage, but how?

			name, ok := guns[gun_type]
			if !ok {
				name = fmt.Sprintf("Unknown (%v)", gun_type)
			}

			mount_name, ok := mounts[mount]
			if !ok {
				mount_name = fmt.Sprintf("Unknown (%v)", mount)
			}
			out += fmt.Sprintf("%v: %v\n", mount_name, name)
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
			lnch_type := int(record.data[i*4])
			mount := int(record.data[i*4+1])
			name, ok := launchers[lnch_type]
			if !ok {
				name = fmt.Sprintf("Unknown (%v)", lnch_type)
			}
			mount_name, ok := mounts[mount]
			if !ok {
				mount_name = fmt.Sprintf("Unknown (%v)", mount)
			}
			out += fmt.Sprintf("%v: %v\n", mount_name, name)
		}

	case "MISL":
		out += "Missiles:\n"
		missiles := map[int]string{
			1: "Torpedo",
			4: "Dumbfire",
			2: "Heat Seeker",
			5: "Image Rec",
			3: "Friend or Foe",
		}
		for i := range len(record.data) / 3 {
			msl_type := int(record.data[i*3])
			count := int(record.data[i*3+1])

			name, ok := missiles[msl_type]
			if !ok {
				name = fmt.Sprintf("Unknown (%v)", msl_type)
			}
			out += fmt.Sprintf("%v: %v\n", name, count)
		}

	case "TRRT":
		out += "Turrets:\n"
		// This only counts turrets, not what you have in them.
		turrets := map[int]string{
			1: "Rear",
			2: "Top",
			3: "Bottom",
		}
		for i := range len(record.data) {
			trrt_type := int(record.data[i])
			name, ok := turrets[trrt_type]
			if !ok {
				name = fmt.Sprintf("Unknown (%v)", trrt_type)
			}
			out += fmt.Sprintf("%v\n", name)
		}

	case "NAVQ", "NNAVQ":
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

	case "CCRGI":
		out += "Cargo-info?:\n"
		// What do credits and cargo expansions have in common?  I'd like to know what they were thinking on this one.
		cur := 0
		out += fmt.Sprintf("Credits: %v\n", read_int_le(record.data, &cur))
		boolmap := map[bool]string{true: "Yes", false: "No"}
		out += fmt.Sprintf("Capacity: %vT, Can have expansion: %v; expanded: %v\n", record.data[4], boolmap[record.data[6] != 0], boolmap[record.data[7] != 0])

	case "RREPR":
		out += "Repair Droid:\n"
		// There doesn't seem to be any variation here
		// TODO: check RF's super repair droid
		expected := []byte{0x90, 1, 0, 0}
		if slices.Equal(record.data, expected) {
			out += "Normal\n"
		} else {
			out += fmt.Sprintf("Unusual Repair Droid!!! Expoected %v; found %v\n", expected, record.data)
		}

	case "AARMR":
		out += "Armour:\n"
		//Shortcut: if it's all 0, you have no armour
		if slices.Equal(record.data, make([]byte, 16, 16)) {
			out += "None\n"
			break
		}

		facings := []string{"Front", "Left", "Right", "Back"}
		names := map[uint8]string{
			0: "Plasteel",
			1: "Tungsten",
		}
		name, ok := names[record.data[1]]
		if !ok {
			name = fmt.Sprintf("Unnknown: %v", record.data[1])
		}
		out += name + "\n"
		// What sees to be happening here is that the first 4 entries are fully repaired values, and the next 4 are current.
		for n, f := range facings {
			// cast to avoid ridiculous overflow issues
			out += fmt.Sprintf("%v: %v%%\n", f, int(record.data[8+2*n])*100/int(record.data[2*n]))
		}

    case "INFO":
		cur:=0
		infotype,_,_ := read_string(record.data, &cur)
		out += "INFO type "+infotype+"\n"
		switch infotype{
			case "SHIELDS":
				out += fmt.Sprintf("Shields level %v\n", int(record.data[cur+1])-89)  //WHY???
			
			default:
				out += fmt.Sprintf("Unknown info type: %v\n", infotype)
		
		}

	case "FORM":
		// Do nothing!  Subforms are handled at the end of the functon.
		
	default:
		out += fmt.Sprintf("(don't know how to parse %v\n", record.name)
	}

	for _, f := range record.forms {
		out += fmt.Sprintf("Found Subform: %v\n", prefix+record.name+"-"+f.name)
		for _, r := range f.records {
			out += parse_record(prefix+record.name+"-"+f.name+"-", r)
			out += "\n"
		}
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

	// The file consists of a fixed-size header, some variable-sized forms, and a footer
	// ...and some mysterious space between the forms.

	header := bytes[0:0x3D]
	fmt.Println("Header:")
	for _, line := range parse_header(header) {
		fmt.Println(line)
	}
	fmt.Println()

	cur := 0x3D
	for cur < len(bytes) {
		form_start := cur
		form, err := read_form(bytes, &cur)

		if err != nil {
			fmt.Println("Error", err, "...bailing")
			break
		}

		fmt.Println(fmt.Sprintf("Form %v at %v - length %v", form.name, form_start, form.length))
		for _, r := range form.records {
			fmt.Println(parse_record(form.name+"-", r))
		}
		fmt.Println(fmt.Sprintf("End of form %v at %v", form.name, cur))
	}

	fmt.Println("Footer:")
	fmt.Println(bytes[cur:])
}
