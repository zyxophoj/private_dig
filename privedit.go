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
// privedit set guns "right:Boosted Steltek gun"
// privedit set guns "left_outer:Boosted Steltek gun"
// privedit set guns "right_outer:Boosted Steltek gun"
// privedit set name Filthy
// privedit set callsign Cheater
// privedit save

import (
	"bufio"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"gopkg.in/ini.v1"

	"privdump/readers"
	"privdump/tables"
	"privdump/types"
	"privdump/writers"
)

var stash_filename = "privedit.tmp"

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

const (
	CT_BLOB = iota
	CT_FORM
	CT_STRING
	CT_STRING_IN_RECORD
)

type ettable struct {
	type_  int
	offset int
	start  int
	end    int

	trans_int map[int]string
	trans_str map[string]string
	record    []string
}

var ettables = map[string]*ettable{

	"ship": &ettable{CT_BLOB, types.OFFSET_SHIP, 0, 1, map[int]string{
		tables.SHIP_TARSUS:    "Tarsus",
		tables.SHIP_ORION:     "Orion",
		tables.SHIP_CENTURION: "Centurion",
		tables.SHIP_GALAXY:    "Galaxy",
	}, map[string]string{}, nil},
	"location": &ettable{CT_BLOB, types.OFFSET_SHIP, 2, 3, make_location_map(), map[string]string{}, nil},
	"credits":  &ettable{CT_FORM, types.OFFSET_REAL, 0, 4, map[int]string{}, map[string]string{}, []string{"FITE", "CRGO", "CRGI"}},
	"shields":  &ettable{CT_FORM, types.OFFSET_REAL, 8, 9, make_shields_map(), map[string]string{}, []string{"FITE", "SHLD", "INFO"}}, // TODO: handle the no-shields case
	"engine":   &ettable{CT_STRING_IN_RECORD, types.OFFSET_REAL, 8, -1, map[int]string{}, make_engine_map(), []string{"FITE", "ENER", "INFO"}},
	"name":     &ettable{CT_STRING, types.OFFSET_NAME, 0, 0, map[int]string{}, map[string]string{}, nil},
	"callsign": &ettable{CT_STRING, types.OFFSET_CALLSIGN, 0, 0, map[int]string{}, map[string]string{}, nil},

	// Evil special cases!
	"guns": &ettable{CT_FORM, types.OFFSET_REAL, 0, -1, map[int]string{}, map[string]string{}, []string{"FITE", "WEAP", "GUNS"}},
	//"launchers": &ettable{CT_FORM, types.OFFSET_REAL, 0, -1, map[int]string{}, map[string]string{}, []string{"FITE", "WEAP", "LNCH"}},
}

func list_ettables() string {
	ret := ""
	for k := range ettables {
		ret = ret + k + "\n"
	}
	return ret
}

// TODO: be more lazy.  Ideally, these make...map functions should not even run if their output is not needed

func make_location_map() map[int]string {
	ret := map[int]string{}

	for id, info := range tables.Bases {
		ret[int(id)] = info.Name
	}

	return ret
}

func make_shields_map() map[int]string {
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
		fmt.Println("You are, at present, helpless.  Sorry about that.")

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

		err = os.Remove(stash_filename)
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

		if len(os.Args) < 4 {
			str := "Set " + what + " to what?  Options are:"
			for _, v := range g.trans_int {
				str += ("\n" + v)
			}
			return errors.New(str)
		}
		to := os.Args[3]

		filename, savedata, err := retrieve()
		if err != nil {
			return err
		}

		err = set(what, to, savedata)
		if err != nil {
			return err
		}

		fmt.Println(what, "set to", to)
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
	bytes, err := ioutil.ReadFile(full_filename)
	if err != nil {
		return nil, err
	}
	header := readers.Read_header(bytes)
	return readers.Read_savedata(header, bytes)
}

func stash(filename string, savedata *types.Savedata) error {
	f, err := os.Create(stash_filename)
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
	f, err := os.Open(stash_filename)
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

// get gets something
// what: the thing to be got
// savedata: processed savefile data
func get(what string, savedata *types.Savedata) (string, error) {
	g, ok := ettables[what]
	if !ok {
		return "", errors.New(what + " is not gettable.    Gettables are:\n" + list_ettables())
	}
	bytes := []uint8{}
	switch g.type_ {
	case CT_STRING:
		return savedata.Strings[g.offset], nil

	case CT_FORM, CT_STRING_IN_RECORD:
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

	// Special case!
	if what == "guns" {
		return get_guns(bytes)
	}

	if g.type_ == CT_STRING_IN_RECORD {
		// TODO: it's starting to look like the CT enum is carrying 2 different kinds of info (the type of thing to be read and what it's in) and should be split
		str := string(bytes)
		return fmt.Sprint(bytes, ": ", g.trans_str[str]), nil
	}

	n := 0
	switch len(bytes) {
	case 1:
		n = int(bytes[0])
	case 4:
		cur := 0
		n = readers.Read_int_le(bytes, &cur)
	default:
		panic("???") // impossible
	}

	if len(g.trans_int) > 0 {
		return fmt.Sprint(n, ": ", g.trans_int[n]), nil
	} else {
		// no translation necessary
		return fmt.Sprint(n), nil
	}
}

// set sets something
// what: the thing to be set
// to: the value to set it to
// savedata: processed savefile data
func set(what string, to string, savedata *types.Savedata) error {
	g, ok := ettables[what]
	if !ok {
		return errors.New(what + " is not settable.  Settables are:\n" + list_ettables())
	}

	if what == "guns" {
		return set_guns(g, to, savedata)
	}

	value := 0
	value_bytes := []byte{}

	if len(g.trans_int) > 0 {
		// The map is for reading. and therefore backwards for writing.
		found := false
		for k, v := range g.trans_int {
			if v == to {
				value = k
				found = true
				break
			}
		}
		if !found {
			// TODO: if that didn't work, try matching more fuzzily
			return errors.New(to + " is not a valid value for " + what)
		}
	} else if len(g.trans_str) > 0 {
		// Another backwards map
		found := false
		for k, v := range g.trans_str {
			if v == to {
				value_bytes = []byte(k)
				found = true
				break
			}
		}
		if !found {
			// TODO: if that didn't work, try matching more fuzzily
			return errors.New(to + " is not a valid value for " + what)
		}
	} else if g.type_ != CT_STRING{
		err := error(nil)
		value, err = strconv.Atoi(to)
		if err != nil {
			return err
		}
	}

	target := []byte{}
	switch g.type_ {
	case CT_STRING:
		// At least this one is easy?
		if len(to) > savedata.Chunk_length(g.offset) {
			// TODO: in "I know what I'm doing" mode, this should just be a warning
			return errors.New(fmt.Sprintf("Failed - new %v has %v characters; max length is %v", what, len(to), savedata.Chunk_length(g.offset)))
		}
		savedata.Strings[g.offset] = to

	case CT_BLOB:
		target = savedata.Blobs[g.offset]

	case CT_FORM:
		target = savedata.Forms[g.offset].Get(g.record...).Data

	case CT_STRING_IN_RECORD:
		record := savedata.Forms[g.offset].Get(g.record...)
		// TODO: deal with nil case
		end := g.end
		if end < 0 {
			end += (len(record.Data) + 1) // +1 because negative indices have to start at -1, not 0
		}

		record.Data = append(record.Data[:g.start], append(value_bytes, record.Data[end:]...)...)
		return nil
	}

	switch g.end - g.start {
	case 0:
		// Nothing to do here.

	case 1:
		target[g.start] = uint8(value)

	case 4:
		writebytes := []byte{uint8(value & 0xff), uint8((value >> 8) & 0xff), uint8((value >> 16) & 0xff), uint8(value >> 24)}
		target[g.start+0] = writebytes[0]
		target[g.start+1] = writebytes[1]
		target[g.start+2] = writebytes[2]
		target[g.start+3] = writebytes[3]

	default:
		panic("???") // impossible
	}

	return nil
}

func safe_lookup[K comparable](from map[K]string, with K) string {
	out, ok := from[with]
	if !ok {
		out = fmt.Sprintf("Unknown (%v)", with)
	}
	return out
}

// Special case code for guns...

func get_guns(data []byte) (string, error) {

	// TODO: this is ripped right out of privdump; unduplicate somehow

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

	// TODO: get game type from savedata (which we do not even have here)

	//if gt == types.GT_PRIV {
	guns[8] = "Steltek Gun"
	// This one lacks an official name, but the Steltek say they attach
	// a power booster, so let's go with that.
	guns[9] = "Boosted Steltek Gun"
	//}
	//if gt == types.GT_RF {
	//	guns[8] = "Fusion Cannon"
	//}

	// TODO: rear/top is "turret 1"; get ship from savedata to make more sense of this?
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

	out := ""
	for i := range len(data) / 4 {
		gun := int(data[i*4])
		mount := int(data[i*4+1])
		damage := int(data[i*4+2])
		d := ""
		if damage > 0 {
			d += " (damaged)"
		}
		// I have no clue what the fourth byte is for.  It always seems to be 0.

		out += fmt.Sprintf("%v: %v\n", safe_lookup(mounts, mount), safe_lookup(guns, gun)) + d
	}

	return out, nil

}

func set_guns(g *ettable, to string, savedata *types.Savedata) error {
	// TODO: this really needs to be unduplicated

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

	// TODO: get game type from savedata

	//if gt == types.GT_PRIV {
	guns[8] = "Steltek Gun"
	// This one lacks an official name, but the Steltek say they attach
	// a power booster, so let's go with that.
	guns[9] = "Boosted Steltek Gun"
	//}
	//if gt == types.GT_RF {
	//	guns[8] = "Fusion Cannon"
	//}

	// TODO: rear/top is "turret 1"; get ship from savedata to make more sense of this?
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

	// decipher "to"
	to_bits := strings.Split(to, ":")
	if len(to_bits) != 2 {
		return errors.New("Expected argument to \"set guns\" is \"mount:gun_type\"")
	}

	// reverse lookup.  bletch.
	to_mount := -1
	for k, v := range mounts {
		if v == to_bits[0] {
			to_mount = k
		}
	}

	if to_mount == -1 {
		return errors.New("Unrecognized mount")
	}
	to_gun := -1
	if to_bits[1] != "empty" {
		for k, v := range guns {
			if v == to_bits[1] {
				to_gun = k
			}
		}
		if to_gun == -1 {
			return errors.New("Unrecognized gun")
		}
	}

	record := savedata.Forms[g.offset].Get(g.record...)
	// TODO: can this be nil?
	data := record.Data
	for i := range len(data) / 4 {
		gun := int(data[i*4])
		mount := int(data[i*4+1])

		if mount == to_mount {
			if to_bits[1] == "empty" {
				fmt.Println("Destroying existing", safe_lookup(guns, gun), "at ", safe_lookup(mounts, mount))
				record.Data = append(record.Data[:4*i], record.Data[4*(i+1):]...)
				return nil
			}
			fmt.Println("Transmogrifying existing", safe_lookup(guns, gun), "at ", safe_lookup(mounts, mount), "into a", safe_lookup(guns, to_gun))
			data[i*4] = uint8(to_gun)
			return nil
		}
	}

	if to_bits[1] == "empty" {
		fmt.Println(safe_lookup(mounts, to_mount), "is already empty, so... done, I guess?")
		return nil
	}

	fmt.Println("Adding new", safe_lookup(guns, to_gun), "at ", safe_lookup(mounts, to_mount))
	record.Data = append(record.Data, uint8(to_gun), uint8(to_mount), 0, 0)
	return nil
}

// sanity_fix attempts to fix inconsistencies in savedata - but only the ones that would cause the game to crash
// Crashing inconsistencies appear to be: weapons or launchers in non-turret mounts that don't exist.
func sanity_fix(savedata *types.Savedata) {
	// Gun mounts: 		1: Left outer, 2: Left, 3: Right, 4: Right outer,
	// Only the Centurion has outer mounts.
	// Launcher mounts: 0: Centre, 1: Left (not Centurion), 2: Left (Centurion), 3: Right (Centurion), 4: Right (not Centurion),
	type fixers struct {
		fix_guns      map[byte]int
		fix_launchers map[byte]int
	}
	mounts := map[uint8]fixers{
		tables.SHIP_TARSUS:    {map[byte]int{1: 2, 4: 3}, map[byte]int{0: -1, 2: 1, 3: 4}},
		tables.SHIP_ORION:     {map[byte]int{1: 2, 4: 3}, map[byte]int{1: -1, 2: -1, 3: -1, 4: -1}},
		tables.SHIP_CENTURION: {map[byte]int{}, map[byte]int{0: -1, 1: 2, 4: 3}},
		tables.SHIP_GALAXY:    {map[byte]int{1: 2, 4: 3}, map[byte]int{0: -1, 2: 1, 3: 4}},
	}

	fix_record := func(weapon string, record *types.Record, fixer map[byte]int) {
		if record == nil {
			// Not an error, sometimes records are empty if there's no equipment
			return
		}

		data := record.Data
		// weapon block format: weapon, mount, damage, ???
		oldmap := map[byte][]byte{}
		for i := range len(data) / 4 {
			oldmap[data[i*4+1]] = data[i*4 : (i+1)*4]
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
					oldmap[mount][1] = byte(new_mount)
					newmap[byte(new_mount)] = oldmap[mount]
				}
			}
		}
		// This does randomize weapon order, but the game doesn't care so neither do I
		record.Data = []byte{}
		for _, gundata := range newmap {
			record.Data = append(record.Data, gundata...)
		}
	}

	ship := savedata.Blobs[types.OFFSET_SHIP][0]
	fix_record("gun", savedata.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "GUNS"), mounts[ship].fix_guns)
	fix_record("launcher", savedata.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH"), mounts[ship].fix_launchers)
}
