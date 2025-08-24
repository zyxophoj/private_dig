package main

// savefile reader/editor for Privateer
//
// example usage:
//
// privedit load new.sav
// privedit set ship Centurion
// privedit set credits 10000000
// privedit set location "New Detroit"
// privedit set engine 5
// privedit set shields 5
// privedit set guns "left:Boosted Steltek gun"
// privedit set guns left_outer:boo
// privedit set guns right:boo
// privedit set guns right_o:boo
// privedit set missiles Image:32000
// privedit set launchers left:miss
// privedit set launchers right:miss
// privedit set launchers turret_1:miss
// privedit set turrets rear:present
// privedit set name Filthy
// privedit set callsign Cheater
// privedit save

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/ini.v1"

	"privdump/readers"
	"privdump/tables"
	"privdump/types"
	"privdump/writers"
)

// Evil global variables
var g_stash_filename = "privedit.tmp"

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

// smash smashes "funny characters" (which includes anything that's remotely tricky to type into a command line) in a string into the '_' character
func smash(in string) string {
	out := ""
	for _, c := range in {
		if unicode.IsLetter(c) || unicode.IsDigit(c) {
			out += string(c)
		} else {
			out += "_"
		}
	}
	return out
}

// string matching functions, in strictly increasing order of desperation
var fuzzy = []func(input string, candidate string) bool{
	func(i string, c string) bool { return i == c },
	func(i string, c string) bool { return strings.ToUpper(i) == strings.ToUpper(c) },
	func(i string, c string) bool { return smash(strings.ToUpper(i)) == smash(strings.ToUpper(c)) },
	func(i string, c string) bool {
		return strings.HasPrefix(smash(strings.ToUpper(c)), smash(strings.ToUpper(i)))
	},
	func(i string, c string) bool {
		return strings.Contains(smash(strings.ToUpper(c)), smash(strings.ToUpper(i)))
	},
}

// Savefile data at every offset is either a form, a string, or a blob.
type ChunkType int

const (
	CT_BLOB ChunkType = iota
	CT_FORM
	CT_STRING
)

type DataType int

const (
	DT_INT DataType = iota
	DT_STRING
	DT_ARRAY
)

type ettable struct {
	chunk_type ChunkType
	data_type  DataType
	offset     int
	start      int
	end        int

	trans_int func(game types.Game) map[int]string
	trans_str map[string]string
	record    []string
}

var ettables = map[string]*ettable{
	"ship":     &ettable{CT_BLOB, DT_INT, types.OFFSET_SHIP, 0, 1, make_ship_map, map[string]string{}, nil},
	"location": &ettable{CT_BLOB, DT_INT, types.OFFSET_SHIP, 2, 3, make_location_map, map[string]string{}, nil},
	"credits":  &ettable{CT_FORM, DT_INT, types.OFFSET_REAL, 0, 4, nil, map[string]string{}, []string{"FITE", "CRGO", "CRGI"}},
	"shield":   &ettable{CT_FORM, DT_INT, types.OFFSET_REAL, 8, 9, make_shields_map, map[string]string{}, []string{"FITE", "SHLD", "INFO"}}, // TODO: handle the no-shields case
	"engine":   &ettable{CT_FORM, DT_STRING, types.OFFSET_REAL, 8, -1, nil, make_engine_map(), []string{"FITE", "ENER", "INFO"}},
	"name":     &ettable{CT_STRING, DT_STRING, types.OFFSET_NAME, 0, 0, nil, map[string]string{}, nil},
	"callsign": &ettable{CT_STRING, DT_STRING, types.OFFSET_CALLSIGN, 0, 0, nil, map[string]string{}, nil},

	// Mountables
	"guns":      &ettable{CT_FORM, DT_ARRAY, types.OFFSET_REAL, 0, -1, make_guns_map, map[string]string{}, []string{"FITE", "WEAP", "GUNS"}},
	"launchers": &ettable{CT_FORM, DT_ARRAY, types.OFFSET_REAL, 0, -1, make_launchers_map, map[string]string{}, []string{"FITE", "WEAP", "LNCH"}},
	"missiles":  &ettable{CT_FORM, DT_ARRAY, types.OFFSET_REAL, 0, -1, nil, map[string]string{}, []string{"FITE", "WEAP", "MISL"}},
	"turrets":   &ettable{CT_FORM, DT_ARRAY, types.OFFSET_REAL, 0, -1, make_present_map, map[string]string{}, []string{"FITE", "TRRT"}},
}

// Extra info for mountables
type mount_info struct {
	mounts           map[int]string
	chunk_length     int
	equipment_offset int
	equipment_length int
	mount_offset     int
}

var mount_infos = map[string]mount_info{
	"guns":      mount_info{tables.Gun_mounts, 4, 0, 1, 1},
	"launchers": mount_info{tables.Launcher_mounts, 4, 0, 1, 1},
	"missiles":  mount_info{tables.Missiles, 3, 1, 2, 0},
	"turrets":   mount_info{tables.Turrets, 1, 0, 0, 0},
}

func list_ettables() string {
	ret := ""
	for k := range ettables {
		ret = ret + k + "\n"
	}
	return ret
}

func make_ship_map(game types.Game) map[int]string {
	return map[int]string{
		tables.SHIP_TARSUS:    "Tarsus",
		tables.SHIP_ORION:     "Orion",
		tables.SHIP_CENTURION: "Centurion",
		tables.SHIP_GALAXY:    "Galaxy",
	}
}

func make_location_map(game types.Game) map[int]string {
	ret := map[int]string{}

	for id, info := range tables.Locations(game) {
		ret[int(id)] = info.Name
	}

	return ret
}

func make_shields_map(game types.Game) map[int]string {
	ret := map[int]string{}

	for n := 1; n < 8; n += 1 {
		ret[n+tables.SHIELD_BASE_0] = strconv.Itoa(n)
	}

	return ret
}

func make_engine_map() map[string]string {
	// TODO: unduplicate this info (it's also in privdump.go)
	pretty := map[string]string{
		"1261":         "0",
		"124151":       "1",
		"12314151":     "2",
		"1231415162":   "3",
		"122131415161": "4",
		"122131415162": "4a",
		"122231415162": "5",
		"122331415162": "6",
		"122431415162": "7",
	}

	// convert key strings so that the actual character value (as an 8-bit int) is the numerical value of the old character
	ugly := map[string]string{}
	for k, v := range pretty {
		new_k := ""
		for _, ch := range k {
			new_k = new_k + string([]byte{byte(ch - '0')}) //UGH!!!
		}
		ugly[new_k] = v
	}
	return ugly
}

func make_guns_map(game types.Game) map[int]string {
	return tables.Guns(game)
}
func make_launchers_map(game types.Game) map[int]string {
	return tables.Launchers
}

func make_present_map(game types.Game) map[int]string {
	return map[int]string{0: "present"}
}

func main() {
	err := main2()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func main2() error {

	arg := "help"
	if len(os.Args) < 2 {
		fmt.Println("No args detected - falling back to \"help\", since you clearly need it...")
	} else {
		arg = os.Args[1]
	}

	switch arg {
	case "help":
		help_text := []string{
			"Privateer Save File Editor",
			"",
			"Commands:",
			"help: display this text",
			"load (filename): load a file from the default location",
			"dump : list all available info",
			"get (what): display current status of something",
			"set (what) (to): set status of something",
			"save: save a file",
			"",
			"Things that can be set-ted or get-ted are:",
		}
		for k := range ettables {
			help_text = append(help_text, "   "+k)
		}
		help_text = append(help_text, []string{
			"",
			"Notes:",
			"   \"empty\" is a magic word.  Where possible, equipment can be removed by",
			"setting it to \"empty\".",
			"   \"present\" is a magic word, used to set equipment that contains no",
			"information beyond its existence.  e.g \"set Turret Rear:present\"",
			"   It is usually not necessary to type the full name of something",
			"e.g. \"new_d\" will be recognized as \"New Detroit\".",
		}...)

		for _, ht := range help_text {
			fmt.Println(ht)
		}

		// TODO: "help(command)" and even "help command what" for extra info.

	case "load":
		if len(os.Args) < 2 {
			return errors.New("Load what?  Filename expected.")
		}

		full_filename := get_dir() + "/" + os.Args[2]
		savedata, err := load(full_filename)
		if err != nil {
			return err
		}

		return stash(full_filename, savedata)

	case "save":
		filename, savedata, err := retrieve()
		if err != nil {
			return err
		}

		sanity_fix(savedata)

		// Back up the old file
		// Since this is a "powerful" (i.e. capable of completely trashing savefiles) tool,
		// that's probably a good idea
		newname := filename[:len(filename)-3] + "old"
		err = os.Rename(filename, newname)
		if err != nil {
			return err
		}
		fmt.Println(filename, "renamed to", newname)

		// The save we were asked to do
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		writer := bufio.NewWriter(f)
		// TODO : catch errors from write_file once it actually emits them
		writers.Write_file(savedata, writer)
		writer.Flush()
		f.Sync()
		fmt.Println("New file written to", filename)
		// It actually isn't written until the deferred Close() happens, but I can live with lying to the user for a few microseconds.

		err = os.Remove(g_stash_filename)
		if err != nil {
			return err
		}
		fmt.Println("Temporary data cleaned up")

	case "get":
		if len(os.Args) < 3 {
			return errors.New("Get what?  Gettables are:\n" + list_ettables())
		}
		what := os.Args[2]

		_, savedata, err := retrieve()
		if err != nil {
			return err
		}

		str, err := get(what, savedata)
		if err != nil {
			return err
		}
		fmt.Println(str)

	case "set":
		if len(os.Args) < 3 {
			return errors.New("Set what? Settables are:\n" + list_ettables())
		}
		what := os.Args[2]

		g, ok := ettables[what]
		if !ok {
			return errors.New(what + " is not settable.  Settables are:\n" + list_ettables()) // UGH! (duplicated in set())
		}

		filename, savedata, err := retrieve()
		if err != nil {
			return err
		}

		if len(os.Args) < 4 {
			str := "Set " + what + " to what?  Options are:"
			for _, v := range g.trans_int(savedata.Game()) {
				str += ("\n" + v)
			}
			return errors.New(str)
		}
		to := os.Args[3]

		to_matched, err := set(what, to, savedata)
		if err != nil {
			return err
		}

		fmt.Println(what, "set to", to_matched)
		return stash(filename, savedata)

	case "dump":
		_, savedata, err := retrieve()
		if err != nil {
			return err
		}

		for what := range ettables {
			str, err := get(what, savedata)
			if err != nil {
				return err
			}
			fmt.Println(what + ":")
			fmt.Println(str)
		}
	}

	return nil
}

func load(full_filename string) (*types.Savedata, error) {
	bytes, err := os.ReadFile(full_filename)
	if err != nil {
		return nil, err
	}
	header := readers.Read_header(bytes)
	sd, err := readers.Read_savedata(header, bytes)
	if err != nil {
		return nil, err
	}
	return sd, err
}

func stash(filename string, savedata *types.Savedata) error {
	f, err := os.Create(g_stash_filename)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	encoder := gob.NewEncoder(w)
	err = encoder.Encode(filename)
	if err != nil {
		return err
	}
	err = encoder.Encode(savedata)
	if err != nil {
		return err
	}
	w.Flush()
	f.Sync()

	return nil
}

func retrieve() (string, *types.Savedata, error) {
	f, err := os.Open(g_stash_filename)
	if err != nil {
		return "", nil, err
	}

	defer f.Close()

	encoder := gob.NewDecoder(bufio.NewReader(f))
	var filename *string
	savedata := types.Savedata{}
	err = encoder.Decode(&filename)
	if err != nil {
		return "", nil, err

	}
	err = encoder.Decode(&savedata)
	if err != nil {
		return "", nil, err
	}

	return *filename, &savedata, nil
}

// fuzzy_reverse_lookup looks up "backwards" in a translation map
//
// trans: map to be looked up in
// to: map value
// what: type of thing to be looked up, as a human-readable string.  Used only in exception construction and probably a mistake
//
// Returns: K: lookup result key, string: lookup result value (not necessarily equal to "to" due to fuzzy matching)
func fuzzy_reverse_lookup[K comparable](trans map[K]string, to string, what string) (K, string, error) {
	var K0 K

	for _, match := range fuzzy {
		matches := []K{}
		names := []string{}
		for k, v := range trans {
			if match(to, v) {
				matches = append(matches, k)
				names = append(names, v)
			}
		}
		if len(matches) == 0 {
			continue
		}
		if len(matches) > 1 {
			return K0, "", errors.New(fmt.Sprint("Ambiguous argument:", to, " could be anything from {", strings.Join(names, ", "), "}"))
		}

		return matches[0], names[0], nil
	}

	return K0, "", errors.New(to + " could not be matched to a valid value for " + what)
}

// get gets something and returns it as a human-readable string
// what: the thing to be got
// savedata: processed savefile data
func get(what string, savedata *types.Savedata) (string, error) {
	g, ok := ettables[what]
	if !ok {
		return "", errors.New(what + " is not gettable.    Gettables are:\n" + list_ettables())
	}
	bytes := []uint8{}
	switch g.chunk_type {
	case CT_STRING:
		return savedata.Strings[g.offset], nil

	case CT_FORM:
		record := savedata.Forms[g.offset].Get(g.record...)
		if record == nil {
			// Not actually an error; sometimes equipment just isn't installed
			return "Nonexistent", nil
		}
		end := g.end
		if end < 0 {
			end += (len(record.Data) + 1) // +1 because negative indices have to start at -1, not 0
		}
		bytes = record.Data[g.start:end]

	case CT_BLOB:
		bytes = savedata.Blobs[g.offset][g.start:g.end]
	}

	if g.data_type == DT_ARRAY {
		return get_mountables(what, bytes, savedata)
	}

	if g.data_type == DT_STRING {
		str := string(bytes)
		return fmt.Sprint(bytes, ": ", g.trans_str[str]), nil
	}

	n, err := read_int(bytes)
	if err != nil {
		return "", err
	}

	if g.trans_int != nil && len(g.trans_int(savedata.Game())) > 0 {
		return fmt.Sprint(n, ": ", g.trans_int(savedata.Game())[n]), nil
	} else {
		// no translation necessary
		return fmt.Sprint(n), nil
	}
}

// set sets something
// Exactly how to set something is encoded in the "ettables" data
// what: the thing to be set
// to: the value to set it to
// savedata: processed savefile data
func set(what string, to string, savedata *types.Savedata) (string, error) {
	g, ok := ettables[what]
	if !ok {
		return "", errors.New(what + " is not settable.  Settables are:\n" + list_ettables())
	}

	if g.data_type == DT_ARRAY {
		return set_mountables(what, to, savedata)
	}

	matched := to
	value := 0
	value_bytes := []byte{}
	if g.trans_int != nil {
		// map is "backwards" from the setting PoV
		v, m, err := fuzzy_reverse_lookup(g.trans_int(savedata.Game()), to, what)
		if err != nil {
			return "", err
		}
		value = v
		matched = m
	} else if len(g.trans_str) > 0 {
		// Another backwards map
		s, m, err := fuzzy_reverse_lookup(g.trans_str, to, what)
		if err != nil {
			return "", err
		}
		value_bytes = []byte(s)
		matched = m

	} else if g.data_type == DT_INT {
		// No lookup available and DT_INT - this is something like "credits" where the expected argument is just an int to be used directly.
		err := error(nil)
		value, err = strconv.Atoi(to)
		if err != nil {
			return "", err
		}
		if value < 0 {
			return "", errors.New("Negative values are not allowed for " + what)
		}
	}

	target := []byte{}
	switch g.chunk_type {
	case CT_STRING:
		// At least this one is easy?
		if len(to) > savedata.Chunk_length(g.offset) {
			// TODO: in "I know what I'm doing" mode, this should just be a warning
			return "", errors.New(fmt.Sprintf("Failed - new %v has %v characters; max length is %v", what, len(to), savedata.Chunk_length(g.offset)))
		}
		savedata.Strings[g.offset] = to

	case CT_BLOB:
		target = savedata.Blobs[g.offset]

	case CT_FORM:
		target = savedata.Forms[g.offset].Get(g.record...).Data
	}

	if g.chunk_type == CT_FORM && g.data_type == DT_STRING {
		record := savedata.Forms[g.offset].Get(g.record...)
		// TODO: deal with nil case?
		end := g.end
		if end < 0 {
			end += (len(record.Data) + 1) // +1 because negative indices have to start at -1, not 0
		}

		record.Data = append(record.Data[:g.start], append(value_bytes, record.Data[end:]...)...)
		return matched, nil
	}

	err := write_int(value, g.end-g.start, target[g.start:g.end])
	if err != nil {
		return "", err
	}

	return matched, nil
}

func safe_lookup[K comparable](from map[K]string, with K) string {
	if from == nil {
		return fmt.Sprint(with)
	}
	out, ok := from[with]
	if !ok {
		out = fmt.Sprintf("Unknown (%v)", with)
	}
	return out
}

func get_mountables(what string, data []byte, savedata *types.Savedata) (string, error) {
	var equipment map[int]string
	if ettables[what].trans_int != nil {
		equipment = ettables[what].trans_int(savedata.Game())
	}
	// TODO: rear/top is "turret 1"; get ship from savedata to make more sense of this?
	mounts := mount_infos[what].mounts

	out := ""
	cl := mount_infos[what].chunk_length
	for i := range len(data) / cl {
		start := i*cl + mount_infos[what].equipment_offset
		end := start + mount_infos[what].equipment_length
		thing, err := read_int(data[start:end])
		if err != nil {
			return "", nil
		}
		mount := int(data[i*cl+mount_infos[what].mount_offset])

		out += fmt.Sprintf("%v: %v\n", safe_lookup(mounts, mount), safe_lookup(equipment, thing))
	}

	return out, nil
}

func set_mountables(what, to string, savedata *types.Savedata) (string, error) {
	g := ettables[what]
	var equipment map[int]string
	if g.trans_int != nil {
		equipment = g.trans_int(savedata.Game())
	}
	mounts := mount_infos[what].mounts

	// decipher "to"
	to_bits := strings.Split(to, ":")
	if len(to_bits) != 2 {
		return "", errors.New("Expected argument to \"set guns\" is \"mount:gun_type\"")
	}

	matched_bits := []string{to_bits[0], to_bits[1]}

	var err error

	to_mount := -1
	to_mount, matched_bits[0], err = fuzzy_reverse_lookup(mounts, to_bits[0], "mount")
	if err != nil {
		return "", err
	}

	to_thing := -1
	if to_bits[1] != "empty" {
		if equipment == nil {
			// no lookup - the number itself is the required value
			to_thing, err = strconv.Atoi(to_bits[1])
			if err != nil {
				return "", err
			}
			// TODO: upper limit depends on mounts.equipment_length
			if to_thing < 1 || to_thing > 32767 {
				return "", errors.New("Numeric argumetn must be between 1 and 32767")
				// TODO: allow 0, treat it the same as "empty"
			}
			matched_bits[1] = to_bits[1]
		} else {
			to_thing, matched_bits[1], err = fuzzy_reverse_lookup(equipment, to_bits[1], what) // TODO un-pluralise "what"?  Ugh.
			if err != nil {
				return "", err
			}
		}
	}

	matched := matched_bits[0] + ":" + matched_bits[1]

	record := savedata.Forms[g.offset].Get(g.record...)
	if record == nil {
		joined := strings.Join(g.record, "-")
		whitelist := map[string]bool{
			"FITE-TRRT": true,
		}
		if whitelist[joined] {
			record = savedata.Forms[g.offset].Add_record(g.record...)
		} else {
			// TODO: construct these things
			return "", errors.New(fmt.Sprintf("Internal privedit error: Unable to construct default(empty) %v record", joined))
		}
	}

	data := record.Data
	minfo := mount_infos[what]
	cl := minfo.chunk_length
	for i := 0; i < len(data); i += cl {
		thing, err := read_int(data[i+minfo.equipment_offset : i+minfo.equipment_offset+minfo.equipment_length])
		if err != nil {
			return "", err
		}
		mount := int(data[i+minfo.mount_offset])

		if mount == to_mount {
			if to_bits[1] == "empty" {
				fmt.Println("Destroying existing", safe_lookup(equipment, thing), "at", safe_lookup(mounts, mount))
				record.Data = append(record.Data[:i], record.Data[i+cl:]...)
				return matched, nil
			}
			fmt.Println("Transmogrifying existing", safe_lookup(equipment, thing), "at", safe_lookup(mounts, mount), "into a", safe_lookup(equipment, to_thing))
			err := write_int(to_thing, minfo.equipment_length, data[i+minfo.equipment_offset:])
			if err != nil {
				return "", err
			}
			return matched, nil
		}
	}

	if to_bits[1] == "empty" {
		fmt.Println(safe_lookup(mounts, to_mount), "is already empty, so... done, I guess?")
		return matched, nil
	}

	fmt.Println("Adding new", safe_lookup(equipment, to_thing), "at", safe_lookup(mounts, to_mount))
	new_data := make([]byte, cl)
	write_int(to_thing, minfo.equipment_length, new_data[minfo.equipment_offset:])
	new_data[minfo.mount_offset] = byte(to_mount)
	record.Data = append(record.Data, new_data...)
	return matched, nil
}

// sanity_fix attempts to fix inconsistencies in savedata - but only the ones that would cause the game to crash
// Crashing inconsistencies appear to be: turrets, weapons or launchers in mounts that don't exist.
func sanity_fix(savedata *types.Savedata) {
	// Turret mounts:   1: Rear, 2:top, 3:bottom
	// Gun mounts: 		1: Left outer, 2: Left, 3: Right, 4: Right outer,
	// Only the Centurion has outer mounts.
	// Launcher mounts: 0: Centre, 1: Left (not Centurion), 2: Left (Centurion), 3: Right (Centurion), 4: Right (not Centurion),
	type fixers struct {
		fix_turrets   map[byte]int
		fix_guns      map[byte]int
		fix_launchers map[byte]int
	}
	// We try to "fix" bad equipment by moving it to a corresponding allowed slot.
	// However, since ships don't even have the same numbers of mounts, weapons
	// must sometimes be thrown away.
	// TODO: try not to throw away steltek gun(s)
	mounts := map[uint8]fixers{
		tables.SHIP_TARSUS:    {map[byte]int{1: -1, 2: -1, 3: -1}, map[byte]int{1: 2, 4: 3}, map[byte]int{0: -1, 2: 1, 3: 4}},
		tables.SHIP_ORION:     {map[byte]int{2: -1, 3: -1}, map[byte]int{1: 2, 4: 3}, map[byte]int{1: -1, 2: -1, 3: -1, 4: -1}},
		tables.SHIP_CENTURION: {map[byte]int{2: -1, 3: -1}, map[byte]int{}, map[byte]int{0: -1, 1: 2, 4: 3}},
		tables.SHIP_GALAXY:    {map[byte]int{1: -1}, map[byte]int{1: 2, 4: 3}, map[byte]int{0: -1, 1: 2, 4: 3}},
	}

	fix_record := func(weapon string, record *types.Record, fixer map[byte]int) {
		if record == nil {
			// Not an error, sometimes records are empty if there's no equipment
			return
		}

		minfo := mount_infos[weapon+"s"] // UGH! GAH!! BLETCH!!!
		cl := minfo.chunk_length

		data := record.Data
		// weapon block format: weapon, mount, damage, ???
		oldmap := map[byte][]byte{}
		for i := range len(data) / cl {
			oldmap[data[i*cl+minfo.mount_offset]] = data[i*cl : (i+1)*cl]
		}
		newmap := map[byte][]byte{}
		for mount := range oldmap {
			new_mount, bad := fixer[mount]
			if !bad {
				// Easy case: gun is allowed to exist
				newmap[mount] = oldmap[mount]
			} else {
				if new_mount == -1 {
					fmt.Println("Sanity fix:", weapon, "from mount", mount, "thrown away")
				} else {
					fmt.Println("Sanity fix:", weapon, "moved from mount", mount, "to mount", new_mount)
					oldmap[mount][minfo.mount_offset] = byte(new_mount)
					newmap[byte(new_mount)] = oldmap[mount]
				}
			}
		}
		// This does randomize weapon order, but the game doesn't care so neither do I
		// TODO: actually, I do care, for file comparison purposes.
		record.Data = []byte{}
		for _, gundata := range newmap {
			record.Data = append(record.Data, gundata...)
		}
	}

	ship := savedata.Blobs[types.OFFSET_SHIP][0]
	fix_record("turret", savedata.Forms[types.OFFSET_REAL].Get("FITE", "TRRT"), mounts[ship].fix_turrets)
	// TODO: add turrets if existing equipment demands it
	fix_record("gun", savedata.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS"), mounts[ship].fix_guns)
	fix_record("launcher", savedata.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH"), mounts[ship].fix_launchers)

	// Engine damage...
	// It looks like engine info is a list of engine subcomponents, and engine damage info is a list of damage-per-subcomponent values.
	// Consequently, a change in engine could result in a change of engine damage info length.  Not doing this seems to cause
	// the game to read off the end of REAL-FITE-ENER-DAMG to find damage values, resulting in nonsensical data and ludicrous repair fees.
	//
	// Just to make things more interesting, REAL-FITE-ENER-DAMG length is not preserved by launch-landing.  We use the longer length here,
	// which is the immediately-after-buying length, mostly because we understand how to calculate it.
	// TODO: understand how to caculate the smaller value, only update if necessary, log iff update happened.
	engine_subcomponents := (len(savedata.Forms[types.OFFSET_REAL].Get("FITE", "ENER", "INFO").Data) - len("ENERGY") - 2) / 2
	savedata.Forms[types.OFFSET_REAL].Get("FITE", "ENER", "DAMG").Data = make([]byte, engine_subcomponents*14)
}

func read_int(bytes []byte) (int, error) {
	n := 0
	switch len(bytes) {
	case 0:
		n = 0
	case 1:
		n = int(bytes[0])
	case 2:
		cur := 0
		n = readers.Read_int16(bytes, &cur)
	case 4:
		cur := 0
		n = readers.Read_int_le(bytes, &cur)
	default:
		return 0, errors.New("Internal privedit error: unexpected byte length for field")
	}

	return n, nil
}

func write_int(n int, length int, target []byte) error {
	switch length {
	case 0:
		if n != 0 {
			return errors.New("Internal privedit error: attempt to write non-zero number to empty")
		}
	case 1, 2, 4: // OK
	default:
		return errors.New("Internal privedit error: unexpected byte length for field")
	}

	for i := 0; i < length; i += 1 {
		target[i] = uint8((n >> (8 * i)) & 0xFF)
	}
	return nil
}
