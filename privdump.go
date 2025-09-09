package main

// privateer save file dumper
// usage: go run privdump.go savefile.sav
//
// save flie location is read from the ini file

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"gopkg.in/ini.v1"

	"privdump/readers"
	"privdump/tables"
	"privdump/types"
	"privdump/utils"
)

func safe_lookup[K comparable](from map[K]string, with K) string {
	out, ok := from[with]
	if !ok {
		out = fmt.Sprintf("Unknown (%v)", with)
	}
	return out
}

func full_location(gt types.Game, id uint8) string {
	loc := tables.Locations(gt)[tables.BASE_ID(id)]
	return loc.Name + " (" + tables.Systems(gt)[loc.System].Name + ")"
}

func parse_savedata(header types.Header, bytes []byte, gt types.Game) []string {
	out := []string{}

	out = append(out, fmt.Sprintf("1-4: File size (%v)", header.File_size))
	cur := 0

	for o := 0; o < len(header.Offsets); o += 1 {
		cur = header.Offsets[o]
		out = append(out, "")
		out = append(out, fmt.Sprintf("%v-%v: %v offset (x%x)", 5+4*o, 8+4*o, types.Offset_name(o), cur))
		switch o {
		case types.OFFSET_SHIP:
			// 0 : ship (0-3), see SHIP constants in tables.go)
			// 1 : Always 0? (It's possible that ship type is actually an int16, but then why is location just a byte?  Editing this does not appear to do anything.)
			// 2 : Location (see Locations in tables.go)
			// 3-4 : Missions completed (int16)
			// 5: Mercenaries guild member (1 or 0)
			// 6: Merchants guild member (1 or 0)
			ships := map[uint8]string{
				tables.SHIP_TARSUS:    "Tarsus",
				tables.SHIP_ORION:     "Orion",
				tables.SHIP_CENTURION: "Centurion",
				tables.SHIP_GALAXY:    "Galaxy",
			}
			ship := readers.Read_uint8(bytes, &cur)
			out = append(out, fmt.Sprintf("   %v: Ship: %v", cur-1, safe_lookup(ships, ship)))

			e := readers.Read_fixed_uint8(bytes, &cur, 0)
			if e != nil {
				out = append(out, fmt.Sprintf("   %v: UNEXPECTED BYTE! ", cur-1)+e.Error())
			}

			loc := readers.Read_uint8(bytes, &cur)
			out = append(out, fmt.Sprintf("   %v: Location: %v", cur-1, full_location(gt, loc)))

			missions := readers.Read_int16(bytes, &cur)
			out = append(out, fmt.Sprintf("   %v-%v: Missions so far: %v", cur-2, cur-1, missions))

			guild_status := map[uint8]string{
				0: "Nonmember",
				1: "Member",
			}
			member := readers.Read_uint8(bytes, &cur)
			out = append(out, fmt.Sprintf("   %v: Mercenaries' Guild: %s", cur-1, safe_lookup(guild_status, member)))
			member = readers.Read_uint8(bytes, &cur)
			out = append(out, fmt.Sprintf("   %v: Merchants' Guild: %s", cur-1, safe_lookup(guild_status, member)))

		case types.OFFSET_PLOT:
			// This is a null-terminated string of max length 8 (9 including the null) with a "flag" byte in the 10th position
			// There are special values for "plot not started" and "plot failed", but otherwise, the string
			// is "s"+(series nubmer)+"m"+(mission letter) e.g. s2mc
			// Note: alphabetical string comparision is CONSIDERED EXTREMELY HARMFUL now that we handle
			// RF missions, which can have a double-digit series number.

			// flag byte is mission status (accepted/done/failed) althoguh this is not well understood.
			status, _, _ := readers.Read_string(bytes, &cur)
			// add 8+1 because this thing is long enough to accommodate the failing "FFFFFFFF" string.
			// There remains one poorly understood byte.
			final := bytes[header.Offsets[o]+8+1 : header.Offsets[o+1]] // This looks like a bitfield
			if status == "" {
				out = append(out, fmt.Sprintf("   (Plot has not been started?)"))
			} else if status == "FFFFFFFF" {
				out = append(out, fmt.Sprintf("   (Plot failed!)"))
			} else {
				// This section begins with something like "s4mb" indicating series and mission number
				serieses := map[string]string{
					"s0": "Sandoval",
					"s1": "Tayla",
					"s2": "Roman Lynch", //and Miggs!
					"s3": "Oxford",
					"s4": "Lynn Murphy",
					"s5": "Taryn Cross",
					"s6": "Goodin?",
					"s7": "Final",

					"s8":  "Tayla (RF)",
					"s9":  "Murphy (RF)",
					"s10": "Goodin (RF)",
					"s11": "Masterson (RF)",
					"s12": "Monte",
					"s13": "Goodin 5 (RF)",
					"s14": "Final", //Patrol Mission of the Apocalypse + Kill Jones?
				}

				// Just to make things more annoying, RF introduces sub-missions.
				// e.g. "Monte 2 (go to Capella)" is "s12mb1", and the informant mission (go to New Detroit) is "s12mb2"
				m := strings.Index(status, "m")
				series, mission := status[0:m], status[m+1:len(status)]
				out = append(out, fmt.Sprintf("   %v - %v", status, final))
				out = append(out, fmt.Sprintf("   Series: %v, Mission %v", safe_lookup(serieses, series), mission))
			}

			mstatus := map[uint8]string{
				128: "Accepted (128)",     //128
				160: "Accepted (160)",     //128+32
				162: "Failed but good",    //128+32+2
				191: "Complete but talky", //128+32+16+8+4+2+1
				226: "Failed",             //128+64+32+2
				255: "Complete",
			}
			// Does this make any sense??
			// "Complete" looks like a special case
			// Here's my best guess:
			// 128: Accepted bit
			// 64: Failed bit
			// 32-1 Objectives bits - mission dependent?

			out = append(out, fmt.Sprintf("   Status: %v", safe_lookup(mstatus, final[0])))
			// This byte can't tell the difference between "You haven't talked to someone yet", and "you talked, rejected, but they'll still be here if you change your mind"
			// That info is in the WTF section

		case types.OFFSET_MISSIONS:
			// 2-bytes, looks like just the mission count
			missions := readers.Read_int16(bytes, &cur)
			out = append(out, fmt.Sprintf("   Non-plot missions: %v", missions))
		case types.OFFSET_WTF:
			out = append(out, fmt.Sprintf("  %v", bytes[cur:header.Offsets[o+1]]))

			// The first 11 bytes appear to be total nonsense - 0% understood right now.
			// They are not even preserved by loadsaving.
			cur += 11

			// Next we have a bunch of flags.  Meanings are not preserved between priv and RF
			flags := utils.Make_flags()

			for i := 0; i < header.Offsets[o+1]-cur; i += 1 {
				if bytes[cur+i] != 0 {
					out = append(out, fmt.Sprintf("  flag %v (%v): %v", i, safe_lookup(flags[gt], i), bytes[cur+i]))
				}
			}

		case types.OFFSET_PLAY, types.OFFSET_SSSS, types.OFFSET_REAL:
			// It's just a form...
			form, err := readers.Read_form(bytes, &cur)
			if err != nil {
				out = append(out, fmt.Sprintf("Bad form!  error:%v", err))
				break
			}
			out = append(out, parse_form("", form, gt)...)

		case types.OFFSET_NAME, types.OFFSET_CALLSIGN:
			// name and callsign are null-terminated strings.
			s, _, _ := readers.Read_string(bytes, &cur)
			out = append(out, fmt.Sprintf("   %v: %v", types.Offset_name(o), s))
		}
	}

	// OK, now do missions
	for m := 0; m < (len(header.Offsets)-types.OFFSET_COUNT)/2; m += 1 {
		cur = header.Offsets[types.OFFSET_COUNT+2*m]
		name, _, _ := readers.Read_string(bytes, &cur)
		out = append(out, fmt.Sprintf("[%v] Mission %v name: %v", header.Offsets[types.OFFSET_COUNT+2*m], m+1, name))

		out = append(out, fmt.Sprintf("[%v] Mission %v data... ", header.Offsets[types.OFFSET_COUNT+2*m+1], m+1))
		cur = header.Offsets[types.OFFSET_COUNT+2*m+1]
		// Another form!
		form, err := readers.Read_form(bytes, &cur)
		if err != nil {
			out = append(out, fmt.Sprintf("Bad form!  error:%v", err))
			break
		}

		out = append(out, parse_form("", form, gt)...)
	}

	return out
}

func parse_form(prefix string, form types.Form, gt types.Game) []string {
	out := []string{}
	out = append(out, "Form "+form.Name)
	for _, r := range form.Records {
		record := parse_record(prefix+form.Name+"-", r, gt)
		for k := range record {
			record[k] = "   " + record[k]
		}
		out = append(out, record...)
	}

	//fmt.Println("SUBFORMS")
	prefix += (form.Name + "-")
	for _, f := range form.Subforms {
		subform := parse_form(prefix, f, gt)
		//fmt.Println("SUBFORM", f.Name)
		for k := range subform {
			subform[k] = "   " + subform[k]
		}
		out = append(out, subform...)
	}

	out = append(out, "End of Form "+form.Name)
	return out
}

func parse_record(prefix string, record types.Record, gt types.Game) []string {
	out := []string{}

	// Record format depends on record name
	if record.Name != "FORM" {
		out = append(out, "Record: "+prefix+record.Name+fmt.Sprintf("%v (%v)", record.Data, len(record.Data)))
	}

	switch record.Name {
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

		out = append(out, "Reputation:")
		for i := range tables.Factions {
			cur := 2 * i
			v := readers.Read_int16(record.Data, &cur)
			if tables.Factions[i] != "" {
				out = append(out, fmt.Sprintf("%-10s: %5v (%s)", tables.Factions[i], v, status(v)))
			}
		}

	case "KILL":
		// Kill count can be displayed on the in-game computer and it behaves correctly for special enemies
		// (e.g. Black Rhombus is a pirate not a merchant, Mordecai Jones is a retro not a hunter)
		out = append(out, "Kills:")
		for i := range tables.Factions {
			cur := 2 * i
			v := readers.Read_int16(record.Data, &cur)
			if tables.Factions[i] != "" || v > 0 {
				out = append(out, fmt.Sprintf("%-10s: %3v", tables.Factions[i], v))
			}
		}

	case "ORIG":
		out = append(out, "Originally Hidden Jump Points:")
		// This is baffling.  Why is starting world state in the save file?  Surely it's only current world state thtat matters.
		// (Maybe record-saving wasn't supported so they had to throw in the whole form?)
		cur := 0
		for cur < len(record.Data) {
			from := tables.SYS_ID(readers.Read_uint8(record.Data, &cur))
			to := tables.SYS_ID(readers.Read_uint8(record.Data, &cur))
			out = append(out, fmt.Sprintf("%v <-> %v", tables.Systems(gt)[from].Name, tables.Systems(gt)[to].Name))
		}

	case "SECT":
		out = append(out, "Hidden Jump Points:")
		cur := 0
		for cur < len(record.Data) {
			from := tables.SYS_ID(readers.Read_uint8(record.Data, &cur))
			to := tables.SYS_ID(readers.Read_uint8(record.Data, &cur))
			out = append(out, fmt.Sprintf("%v <-> %v", tables.Systems(gt)[from].Name, tables.Systems(gt)[to].Name))
		}
		// There is some strangeness here.  This record is often one jump point behind reality.
		// Launching and landing will generally fix this - perhaps things get updated at launch but there's no way to test this.

	case "GUNS":
		out = append(out, "Guns:")
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
		}

		if gt == types.GT_PRIV {
			guns[8] = "Steltek Gun"
			// This one lacks an official name, but the Steltek say they attach
			// a power booster, so let's go with that.
			guns[9] = "Boosted Steltek Gun"
		}
		if gt == types.GT_RF {
			guns[8] = "Fusion Cannon"
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
		for i := range len(record.Data) / 4 {
			gun := int(record.Data[i*4])
			mount := int(record.Data[i*4+1])
			damage := int(record.Data[i*4+2])
			d := ""
			if damage > 0 {
				d += " (damaged)"
			}
			// I have no clue what the fourth byte is for.  It always seems to be 0.

			out = append(out, fmt.Sprintf("%v: %v", safe_lookup(mounts, mount), safe_lookup(guns, gun))+d)
		}

	case "LNCH":
		out = append(out, "Launchers:")

		for i := range len(record.Data) / 4 {
			launcher := int(record.Data[i*4])
			mount := int(record.Data[i*4+1])
			out = append(out, fmt.Sprintf("%v: %v", safe_lookup(tables.Launcher_mounts, mount), safe_lookup(tables.Launchers, launcher)))
		}

	case "MISL":
		out = append(out, "Missiles:")
		missiles := map[uint8]string{
			1: "Torpedo",
			4: "Dumbfire",
			2: "Heat Seeker",
			5: "Image Rec",
			3: "Friend or Foe",
		}
		cur := 0
		for i := range len(record.Data) / 3 {
			msl_type := record.Data[i*3]
			cur += 1
			// Yes, really.  With a maximum of 20 missiles total, a normal game won't need the second byte of this int,
			// but it is used.  Hex edit yourself up to 32767 missiles if you want.
			count := readers.Read_int16(record.Data, &cur)
			out = append(out, fmt.Sprintf("%v: %v", safe_lookup(missiles, msl_type), count))
		}

	case "TRRT":
		out = append(out, "Turrets:")
		// This only counts turrets, not what you have in them.
		for i := range len(record.Data) {
			out = append(out, fmt.Sprintf("%v", safe_lookup(tables.Turrets, int(record.Data[i]))))
		}

	case "NAVQ":
		// This one's a bitfield that tracks which of the 4 quadrant maps we have.
		maps := map[uint8]string{
			1 << tables.QUAD_HUMBOLDT: "Humboldt",
			1 << tables.QUAD_FARISS:   "Fariss",
			1 << tables.QUAD_POTTER:   "Potter",
			1 << tables.QUAD_CLARKE:   "Clarke",
		}

		out = append(out, "Maps:")
		// Short description for the overwhelmingly most common case
		if record.Data[0] == 15 {
			out = append(out, "All")
			break
		}
		for k, v := range maps {
			if k&record.Data[0] != 0 {
				out = append(out, v)
			}
		}

	case "AFTB":
		// A 0-length record.  Either you have afterburners or you don't.
		out = append(out, "Afterburners")
		out = append(out, "(present)")

	case "ECMS":
		// A 1-length record.
		// The manual states that the 3 levels of ECM are 25%, 50% and 75% effective.
		// It looks like the ID here is doing double duty as effectiveness%.
		out = append(out, "ECM:")
		out = append(out, fmt.Sprintf("%v%% effective", record.Data[0]))

	case "CRGI":
		out = append(out, "Cargo-info?:")
		// What do credits and cargo expansions have in common?  I'd like to know what they were thinking on this one.
		cur := 0
		out = append(out, fmt.Sprintf("Credits: %v", readers.Read_int_le(record.Data, &cur)))
		boolmap := map[bool]string{true: "Yes", false: "No"}
		out = append(out, fmt.Sprintf("Capacity: %vT, Secret compartment: %v; expanded: %v", record.Data[4], boolmap[record.Data[6] != 0], boolmap[record.Data[7] != 0]))

	case "REPR":
		out = append(out, "Repair Droid:")
		droids := map[int]string{
			400: "Repair Droid",
			200: "Advanced Droid",
		}
		cur := 0
		out = append(out, fmt.Sprintf("%v", safe_lookup(droids, readers.Read_int16(record.Data, &cur))))

		// 2 0 bytes?
		for cur < 4 {
			e := readers.Read_fixed_uint8(record.Data, &cur, 0)
			if e != nil {
				out = append(out, fmt.Sprintf("   %v: UNEXPECTED BYTE! ", cur-1)+e.Error())
			}
		}

	case "ARMR":
		// 16 bytes, which looks like 8 16-bit ints.
		// First 4 entries are fully-repaired values - which also seem to be doing double duty as armor type.
		// These are always the same, so we only bother with the first.
		// Selling armour - or buying a new ship - results in armour type 0, but launch-landing changes that armour type to 1.
		// (maybe they didn't want to divide by 0, which they would normally have to do to show armour %age).  In any case,
		// that means there are two different types of nothing that we have to deal with.  Sigh.
		cur := 0
		out = append(out, "Armour:")
		names := map[int]string{
			0:    "none (0)",
			1:    "none (1)",
			250:  "Plasteel",
			500:  "Tungsten",
			3000: "Isometal", // Yes, really.  RF is bonkers.
		}
		armor_type := readers.Read_int16(record.Data, &cur)
		out = append(out, fmt.Sprintf("Armour type:%v", safe_lookup(names, armor_type)))
		cur += 6

		// Avoid calculating percentages when they are 0/0 !!
		if armor_type == 0 {
			out = append(out, "None")
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
			out = append(out, fmt.Sprintf("%v: %v%%", f, readers.Read_int16(record.Data, &cur)*100/armor_type))
		}

	case "INFO":
		cur := 0

		if strings.HasSuffix(prefix, "JDRV-") {
			out = append(out, "Jump drive info")
			out = append(out, fmt.Sprintf("Jumps: %v", readers.Read_int16(record.Data, &cur)))
			out = append(out, fmt.Sprintf("Capacity?: %v", readers.Read_int16(record.Data, &cur)))
			break
		}

		if strings.HasSuffix(prefix, "TRGT-") {
			readers.Read_fixed_string("TARGETNG", record.Data, &cur)

			scanner := readers.Read_uint8(record.Data, &cur) - 60 //Why 60???

			names := []string{"Iris Mk I", "Iris Mk II", "Iris Mk III",
				"Hunter AW 6", "Hunter Aw 6i", "Hunter Aw Inf",
				"BS Tripwire", "B.S.  E.Y.E", "B.S. Omni"}

			// Like anyone cares about the names.  Scanner capabilities are determined by position in the 3x3 grid.
			lockiness := []string{"No Lock", "Lock", "Lock, ITTS"}
			colorosity := []string{"All Grey", "Colour", "Full Colour"}

			if scanner < 0 || scanner >= 9 {
				out = append(out, fmt.Sprintf("UNEXPECTED SCANNER!! (%v)", scanner))
				break
			}

			out = append(out, fmt.Sprintf("Scanner: %v (%v, %v)", names[scanner], colorosity[scanner/3], lockiness[scanner%3]))
			break
		}

		infotype, _, _ := readers.Read_string(record.Data, &cur)
		cur -= 1
		out = append(out, "INFO type "+infotype)
		switch infotype {
		case "SHIELDS":
			out = append(out, fmt.Sprintf("Shields level %v", int(record.Data[cur+1])-89)) //WHY???
		case "ENERGY":
			d := record.Data[len("ENERGY")+2 : len(record.Data)]
			strd := ""
			for _, n := range d {
				strd += fmt.Sprintf("%v", n)
			}
			// Yes, really.  There is clearly some structure in here, but I can't make any sense out of it.
			// As a practical matter, you can edit level 7 engines into a Centurion and have an absurdly overpowered ship,
			// but the things that you'd expect to appear next in the list (like "122531415162" are worse than level 2 engines.
			levels := map[string]string{
				"1261":         "(None)",
				"124151":       "Level 1",
				"12314151":     "Level 2",
				"1231415162":   "Level 3",
				"122131415161": "Level 4",
				"122131415162": "Level 4",
				"122231415162": "Level 5",
				"122331415162": "Level 6",
				"122431415162": "Level 7",
			}

			// This calculation is on probation
			// Since it only uses a small amount of the information on offer, it's probably not completely correct.
			dirty := len(strd)/2 - 2
			if strd[2] == '2' {
				dirty += int(strd[3] - '1')
			}

			out = append(out, fmt.Sprintf("Engine: %v", safe_lookup(levels, strd)))
			out = append(out, fmt.Sprintf("Dirty Value: %v", dirty))
		default:
			out = append(out, fmt.Sprintf("Unknown info type: %v", infotype))

		}

	case "DATA":
		// Each 4-byte block is: cargo-type, quantity(2 bytes!), hiddenness
		// Note that if illegal cargo spills out into the non-secret area, it will have 2 entries here.
		// Maximum cargo chunk size would appear to be 255T (Galaxy with cargo upgrade), so it's not clear why
		// we need 16 bits here (maybe there were ships with larger capacity in development?)
		out = append(out, "Cargo data:")
		for cur := 0; cur < len(record.Data); {
			cargo := readers.Read_uint8(record.Data, &cur)
			quantity := readers.Read_int16(record.Data, &cur)
			hidden := readers.Read_uint8(record.Data, &cur)

			hiddenness := map[uint8]string{
				0: "",
				1: " (hidden)",
			}
			out = append(out, fmt.Sprintf("%v (%vT)%v", safe_lookup(tables.Cargo, cargo), quantity, safe_lookup(hiddenness, hidden)))
		}

	case "TEXT":
		// Mission text that displays in the in-game computer.  It's just ASCII text.
		out = append(out, "")
		out = append(out, string(record.Data[1:]))

	case "CARG":
		// Mission cargo - a 3-byte field...
		// Byte 0: destination
		// Byte 1: always 49 - this could be cargo type, since missions are always "mission cargo", even when the descriptions say they are not.
		// Byte 2: How many tons
		out = append(out, fmt.Sprintf("Deliver %vT of %v to %v", record.Data[2], safe_lookup(tables.Cargo, record.Data[1]), tables.Locations(gt)[tables.BASE_ID(record.Data[0])]))

	case "PAYS":
		//Mission payment (4 bytes, although I've never seen a mission that needed all that)
		cur := 0
		pays := readers.Read_int_le(record.Data, &cur)
		out = append(out, fmt.Sprintf("%v credits", pays))

	case "COOL":
		out = append(out, "Gun Cooler")

	case "SHBO":
		out = append(out, "Shield Regenerator")

	case "SPEE":
		out = append(out, "Speed Enhancer")

	case "THRU":
		out = append(out, "Thrust Enhancer")

	case "FORM":
		// Do nothing!  Subforms are already handled in parse_form.

	default:
		out = append(out, fmt.Sprintf("(don't know how to parse %v)", record.Name))
	}

	if len(record.Footer) > 0 {
		out = append(out, fmt.Sprintf("Record has footer: %v", record.Footer))
	}

	return out
}

func get_dir() string {
	// dir from command line
	if len(os.Args) > 1 && os.Args[1] == "--dir" {
		return os.Args[2]
	}

	//dir from ini file
	cfg, err := ini.Load("priv_ach.ini")
	if err == nil {
		// Classic read of values, default section can be represented as empty string
		dir := cfg.Section("").Key("dir").String()
		if dir != "" {
			return dir
		}
	}

	wd, _ := os.Getwd()
	return wd
}

func main() {

	basedir := get_dir()

	filename := os.Args[1]
	full_filename := basedir + "/" + filename

	dot := strings.LastIndex(full_filename, ".")
	if dot < 0 {
		fmt.Println("Failed to load file", full_filename, ": file has no extension")
		os.Exit(1)
	}
	gt := map[string]types.Game{
		".SAV": types.GT_PRIV,
		".PRS": types.GT_RF,
	}[strings.ToUpper(full_filename[dot:])]

	if gt == types.GT_NONE {
		fmt.Println("Failed to load file", full_filename, ": file extension not recognised")
		os.Exit(1)
	}

	bytes, err := ioutil.ReadFile(full_filename)
	if err != nil {
		fmt.Println("Failed to load file", full_filename, "-", err)
		os.Exit(1)
	}

	header := readers.Read_header(bytes)
	fmt.Println()
	for _, line := range parse_savedata(header, bytes, gt) {
		fmt.Println(line)
	}
	fmt.Println()

}
