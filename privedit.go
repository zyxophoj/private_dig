package main

// savefile reader/editor for Privateer
//
// example usage:
/*
privedit load new.sav
privedit set ship Centurion
privedit set credits 10000000
privedit set location "New Detroit"
privedit set engine 5
privedit set shield 5
privedit set guns "left:Boosted Steltek gun"
privedit set guns left_outer:boo right:boo right_o:boo
privedit set missiles Image:32000
privedit set launchers left:miss right:miss turret_1:miss
privedit set turrets rear:present
privedit set reputation retros:100
privedit set name Filthy
privedit set callsign Cheater
privedit save
*/

import (
	"bufio"
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/ini.v1"

	"privdump/burstlogger"
	"privdump/readers"
	"privdump/tables"
	"privdump/types"
)

// Evil global variables
var g_stash_filename = "privedit.tmp"

func get_dir() string {
	// dir from command line
	if len(os.Args) > 2 && os.Args[1] == "--dir" {
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

// zero returns the zero value for a type
func zero[K any]() K {
	var k K
	return k
}

// sniff in an inexacronym for "Safe Nil Function"
// Although nil maps, slices and arrays act like non-modifiable empty objects of the same type,
// nil functions act like nuclear-armed landmines.  A sniff-ed function just returns the default value
// if the function is nil.
func sniff[K any](fn func() K) func() K {
	return func() K {
		if fn == nil {
			return zero[K]()
		}
		return fn()
	}
}

// sniff1 is sniff for one-argument functions
func sniff1[K any, A any](fn func(arg A) K) func(arg A) K {
	return func(a A) K {
		if fn == nil {
			return zero[K]()
		}
		return fn(a)
	}
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

type Logger interface {
	Logln(a ...any)
	Logfn(str string, strs ...any)
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
	DT_INT DataType = 1 << iota
	DT_STRING
	DT_HASMOUNT  // Block of data, including a mount field
	DT_ADDMOUNT  // Block of data, no explicit mount field because position is the mount
	DT_ALLOW_NUM // Allow raw numbers, even though a map is present
	DT_FIXED     // U8.8 fixed point.  This is treated as a presentation and input issue only - internally, we just muiltiply by 256 and deal with ints.
)

// ET_* things and "ettables" are (s)ettable or (g)ettable things
type etype int

const (
	ET_NONE etype = iota // not a real value

	ET_SHIP
	ET_LOCATION
	ET_CREDITS
	ET_SHIELD
	ET_ENGINE
	ET_NAME
	ET_CALLSIGN

	ET_GUN
	ET_LAUNCHER
	ET_MISSILE //including torpedos
	ET_TURRET
	ET_REPUTATION
	ET_KILLS
	ET_CARGO

	ET_SPEED_UP
	ET_THRUST_UP
	ET_SHIELD_UP
	ET_GUN_COOLER

	ET_COUNT
)

type ettable struct {
	chunk_type   ChunkType
	data_type    DataType
	offset       int // chunk offset within file from the OFFSET_* enum
	start        int // data offset within chunk or record
	end          int // data offset within chunk or record
	int_min      int // Only for non-looked-up DT_INT values
	int_max      int // likewise
	can_be_empty bool
	trans_int    func(game types.Game) map[int]string
	trans_str    map[string]string
	record       []string // record within chunk (only if chunk_type is CT_FORM)
	hr_name      string
	rf_only      bool
}

// Extra info for mountables
type mount_info struct {
	mounts           map[int]string
	chunk_length     int
	equipment_offset int
	equipment_length int
	mount_offset     int
	hr_name_plural   string
}

func map_from_array[K comparable](in []K) map[int]K {
	out := map[int]K{}
	for i, v := range in {
		out[i] = v
	}
	return out
}

// Savefile format data starts

var ettables = map[etype]*ettable{
	ET_SHIP:     &ettable{CT_BLOB, DT_INT, types.OFFSET_SHIP, 0, 1, 0, 0, false, make_ship_map, map[string]string{}, nil, "ship", false},
	ET_LOCATION: &ettable{CT_BLOB, DT_INT, types.OFFSET_SHIP, 2, 3, 0, 0, false, make_location_map, map[string]string{}, nil, "location", false},
	ET_CREDITS:  &ettable{CT_FORM, DT_INT, types.OFFSET_REAL, 0, 4, 0, math.MaxInt32, false, nil, map[string]string{}, []string{"FITE", "CRGO", "CRGI"}, "credits", false},
	ET_SHIELD:   &ettable{CT_FORM, DT_INT, types.OFFSET_REAL, 8, 9, 0, 0, true, make_shields_map, map[string]string{}, []string{"FITE", "SHLD", "INFO"}, "shield", false},
	ET_ENGINE:   &ettable{CT_FORM, DT_STRING, types.OFFSET_REAL, 8, -1, 0, 0, false, nil, make_engine_map(), []string{"FITE", "ENER", "INFO"}, "engine", false},
	ET_NAME:     &ettable{CT_STRING, DT_STRING, types.OFFSET_NAME, 0, 0, 0, 0, false, nil, nil, nil, "name", false},
	ET_CALLSIGN: &ettable{CT_STRING, DT_STRING, types.OFFSET_CALLSIGN, 0, 0, 0, 0, false, nil, nil, nil, "callsign", false},

	// Mountables
	ET_GUN:        &ettable{CT_FORM, DT_INT | DT_HASMOUNT, types.OFFSET_REAL, 0, -1, 0, 0, true, make_guns_map, map[string]string{}, []string{"FITE", "WEAP", "GUNS"}, "gun", false},
	ET_LAUNCHER:   &ettable{CT_FORM, DT_INT | DT_HASMOUNT, types.OFFSET_REAL, 0, -1, 0, 0, true, make_launchers_map, map[string]string{}, []string{"FITE", "WEAP", "LNCH"}, "launcher", false},
	ET_MISSILE:    &ettable{CT_FORM, DT_INT | DT_HASMOUNT, types.OFFSET_REAL, 0, -1, 0, 32767, true, nil, map[string]string{}, []string{"FITE", "WEAP", "MISL"}, "missile", false},
	ET_TURRET:     &ettable{CT_FORM, DT_INT | DT_HASMOUNT, types.OFFSET_REAL, 0, -1, 0, 0, true, present(0), map[string]string{}, []string{"FITE", "TRRT"}, "turret", false},
	ET_REPUTATION: &ettable{CT_FORM, DT_INT | DT_ADDMOUNT, types.OFFSET_PLAY, 0, -1, -32767, 32767, false, nil, map[string]string{}, []string{"SCOR"}, "reputation", false},
	ET_KILLS:      &ettable{CT_FORM, DT_INT | DT_ADDMOUNT, types.OFFSET_PLAY, 0, -1, 0, 65535, false, nil, map[string]string{}, []string{"KILL"}, "kills", false},
	ET_CARGO:      &ettable{CT_FORM, DT_INT | DT_HASMOUNT, types.OFFSET_REAL, 0, -1, 0, 0, false, nil, map[string]string{}, []string{"FITE", "CRGO", "DATA"}, "cargo", false},

	ET_SPEED_UP:   &ettable{CT_FORM, DT_INT | DT_ALLOW_NUM | DT_FIXED, types.OFFSET_REAL, 0, 2, 0, 32767, true, present(300), nil, []string{"FITE", "SPEE"}, "speed enhancer", true},
	ET_THRUST_UP:  &ettable{CT_FORM, DT_INT | DT_ALLOW_NUM | DT_FIXED, types.OFFSET_REAL, 0, 2, 0, 32767, true, present(300), nil, []string{"FITE", "THRU"}, "thrust enhancer", true},
	ET_SHIELD_UP:  &ettable{CT_FORM, DT_INT | DT_ALLOW_NUM | DT_FIXED, types.OFFSET_REAL, 0, 2, 0, 32767, true, present(320), nil, []string{"FITE", "SHBO"}, "shield regenerator", true},
	ET_GUN_COOLER: &ettable{CT_FORM, DT_INT | DT_ALLOW_NUM | DT_FIXED, types.OFFSET_REAL, 0, 2, 0, 32767, true, present(320), nil, []string{"FITE", "COOL"}, "gun cooler", true},
}

var mount_infos = map[etype]mount_info{
	ET_GUN:        mount_info{tables.Gun_mounts, 4, 0, 1, 1, "guns"},
	ET_LAUNCHER:   mount_info{tables.Launcher_mounts, 4, 0, 1, 1, "launchers"},
	ET_MISSILE:    mount_info{tables.Missiles, 3, 1, 2, 0, "missiles"},
	ET_TURRET:     mount_info{tables.Turrets, 1, 0, 0, 0, "turrets"},
	ET_REPUTATION: mount_info{map_from_array(tables.Factions), 2, 0, 2, 0, "reputation"},
	ET_KILLS:      mount_info{map_from_array(tables.Factions), 2, 0, 2, 0, "kills"},
	ET_CARGO:      mount_info{tables.Cargo, 4, 1, 2, 0, "cargo"},
}

// add_new_record adds a new record to a savadata
// Ideally, this should be a member function of types.Savedata, but that would make a promise of
// completeness that really isn't delivered here.  This is the "good enough for privedit" version.
//
// offset is the data offset- which had better be a form offset - where the target form is located
// name is a list of record names - including the name of the record to be created in the last position
// Since forms can be nested, this is needed to specify a record location)
func add_new_record(savedata *types.Savedata, offset int, name []string) (*types.Record, error) {
	joined := strings.Join(name, "-")

	// Theoretical record data indicating "no equipment" or containing a blank space for equipment data to go in
	// this is often actually empty, but sometimes the game uses several bytes to say "nothing"
	// These are arguably invalid until equipment data has been added, because if the game actually used an empty record
	// rather than "no record", they wouldn't be here.
	empties := map[string][]byte{
		"FITE-TRRT":      nil,
		"FITE-WEAP-GUNS": nil,
		"FITE-WEAP-LNCH": nil,
		"FITE-WEAP-MISL": nil,
		"FITE-SHLD-INFO": []byte{'S', 'H', 'I', 'E', 'L', 'D', 'S', 0, 0},
		"FITE-SHLD-DAMG": []byte{0, 0},
		"FITE-SPEE":      []byte{44, 1}, //300
		"FITE-THRU":      []byte{44, 1},
		"FITE-SHBO":      []byte{64, 1}, //320
		"FITE-COOL":      []byte{64, 1},
	}
	if data, ok := empties[joined]; ok {
		record := savedata.Forms[offset].Add_record(name...)
		record.Data = data
		return record, nil
	}
	return nil, errors.New(fmt.Sprintf("Internal privedit error: Unable to construct default(empty) %v record", joined))
}

// Savefile format data end
func etype_from_string(str string) etype {
	out := ET_NONE
	for k, v := range ettables {
		if str == v.hr_name {
			out = k
		}
	}
	if out != ET_NONE {
		return out
	}
	for k, v := range mount_infos {
		if str == v.hr_name_plural {
			out = k
		}
	}
	return out
}

func list_ettables() string {
	ret := ""
	for i := range ET_COUNT - 1 {
		v := ettables[i+1]

		ret += v.hr_name
		if v.rf_only {
			ret += " (RF)"
		}
		ret += "\n"
	}
	return ret
}

func make_ship_map(game types.Game) map[int]string {
	// TODO: unduplicate this info (it's also in privdump.go)
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

func present(i int) func(game types.Game) map[int]string {
	return func(game types.Game) map[int]string {
		return map[int]string{i: "present"}
	}
}

func make_human_readable(what etype, value interface{}, game types.Game) string {
	mount_info, is_mountable := mount_infos[what]
	translate := sniff1(ettables[what].trans_int)(game)

	format_info := func(prefix string, et *ettable, value interface{}) string {
		if value == nil {
			return prefix + " (not present)"
		}
		if (et.data_type & DT_FIXED) != 0 {
			return fmt.Sprintf("%v%v", prefix, safe_lookup_fixed(translate, value.(int)))
		}

		if (et.data_type & DT_INT) != 0 {
			return fmt.Sprintf("%v%v    ", prefix, safe_lookup(translate, value.(int)))
		}

		return fmt.Sprintf("%v%v", prefix, safe_lookup(et.trans_str, value.(string)))
	}

	out := ""
	if is_mountable {
		out += ettables[what].hr_name + ": "
		for mount, subvalue := range value.(map[int]interface{}) {
			out += format_info(safe_lookup(mount_info.mounts, mount)+":", ettables[what], subvalue)
		}
	} else {
		out += format_info(ettables[what].hr_name+": ", ettables[what], value)
	}
	return out
}

// main1  makes sure we exit with the right code
func main() {
	bl := burstlogger.BurstLogger{}
	log := &bl

	err := main2(log)
	if err != nil {
		os.Exit(1)
	}
}

// main2 makes sure all logs are written
// (sadly, this can't be combined with main1 because os.Exit destroys deferred calls)
func main2(log *burstlogger.BurstLogger) error {
	defer log.Fire()
	err := main3(log)
	if err != nil {
		log.Logln(err)
	}
	return err
}

// main3 is the real main function
func main3(log *burstlogger.BurstLogger) error {
	arg := "help"
	if len(os.Args) < 2 {
		log.Logln("No args detected - falling back to \"help\", since you clearly need it...")
		log.Fire() // Get that out before help test
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
		help_text = append(help_text, list_ettables())
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

		sanity_fix(savedata, log)

		// Back up the old file
		// Since this is a "powerful" (i.e. capable of completely trashing savefiles) tool,
		// that's probably a good idea
		newname := filename[:len(filename)-3] + "old"
		err = os.Rename(filename, newname)
		if err != nil {
			return err
		}
		log.Logln(filename, "renamed to", newname)

		// The save we were asked to do
		f, err := os.Create(filename)
		if err != nil {
			return err
		}
		defer f.Close()
		writer := bufio.NewWriter(f)
		// TODO : catch errors from Write once it actually emits them
		savedata.Write(writer)
		writer.Flush()
		f.Sync()
		log.Logln("New file written to", filename)

		err = os.Remove(g_stash_filename)
		if err != nil {
			return err
		}
		log.Logln("Temporary data cleaned up")

	case "get":
		if len(os.Args) < 3 {
			return errors.New("Get what?  Gettables are:\n" + list_ettables())
		}

		what := etype_from_string(os.Args[2])
		if what == ET_NONE {
			return errors.New(os.Args[2] + " is not gettable  Gettables are:\n" + list_ettables())
		}

		filename, savedata, err := retrieve()
		if err != nil {
			return err
		}

		if ettables[what].rf_only && savedata.Game() != types.GT_RF {
			return errors.New(ettables[what].hr_name + " is RF-only, and " + filename + " is not an RF file")
		}

		value, err := get(what, savedata)
		if err != nil {
			return err
		}

		fmt.Println(make_human_readable(what, value, savedata.Game()))

	case "set":
		setargses, err := parseSetArgs(os.Args[2:])
		if err != nil {
			return err
		}

		filename, savedata, err := retrieve()
		if err != nil {
			return err
		}

		for _, set_args := range setargses {
			if ettables[set_args.what].rf_only && savedata.Game() != types.GT_RF {
				return errors.New(ettables[set_args.what].hr_name + " is RF-only, and " + filename + " is not an RF file")
			}

			_, is_mountable := mount_infos[set_args.what]
			if is_mountable {
				err = set_at_mount(set_args.what, set_args.to_value, set_args.to_mount, savedata, log)
			} else {
				err = set(set_args.what, set_args.to_value, savedata, log)
			}
			if err != nil {
				// TODO: think about this
				// If we were given a partially-valid instruction, should we do part of it, or fail entirely?
				// Currently we fail entirely, because one error returns out here so the stash never happens.

				log.Forget() // Since it "didn't happen", don't log it
				return err
			}
			log.Logln(set_args.hr_name, "set to", set_args.matched)
		}

		return stash(filename, savedata)

	case "dump":
		_, savedata, err := retrieve()
		if err != nil {
			return err
		}

		for what := etype(1); what < ET_COUNT; what += 1 {
			if ettables[what].rf_only && savedata.Game() == types.GT_PRIV {
				// This question shouldn't arise
				continue
			}

			value, err := get(what, savedata)
			if err != nil {
				return err
			}
			fmt.Println(make_human_readable(what, value, savedata.Game()))
		}
		fmt.Println()
	default:
		return errors.New(arg + " is not a command")
	}

	return nil
}

type setArgs struct {
	what     etype
	to_value interface{}
	to_mount int
	hr_name  string
	matched  string
}

// parseSetArgs parses arguments to the "Set" function.
// This can only deal with one etype at once, but can have many instructions in a mountable case e.g.
// "set guns left_outer:boo right_outer:boo right:boo right_o:boo" will return an array of 4 setArgs-es.
func parseSetArgs(args []string) (setargses []setArgs, err error) {
	err = func() error {
		if len(args) < 1 {
			return errors.New("Set what?  Settables are:\n" + list_ettables())
		}
		what := etype_from_string(args[0])
		if what == ET_NONE {
			return errors.New(args[0] + " is not settable.  Settables are:\n" + list_ettables())
		}
		info := ettables[what]

		// Ugly, but what else can we do?  We can't validate args without knowing what type of savefile
		// we've been asked to modify, so we can't really avoid the retreive.
		_, savedata, err := retrieve()
		if err != nil {
			return err
		}

		if len(args) < 2 {
			str := "Set " + args[0] + " to what?  Options are:"
			for _, v := range info.trans_int(savedata.Game()) {
				str += ("\n" + v)
			}
			return errors.New(str)
		}
		to_list := args[1:]

		_, is_mountable := mount_infos[what]
		if len(to_list) > 1 && !is_mountable {
			return errors.New(args[0] + " can only be set to one thing!")
		}

		//success := []string{}
		for _, to := range to_list {
			// decipher "to"
			matched := ""
			mount_matched := ""

			to_mount := -1
			if is_mountable {
				mounts := mount_infos[what].mounts

				to_bits := strings.Split(to, ":")
				if len(to_bits) != 2 {
					return errors.New("Expected argument to \"set " + info.hr_name + "\" is \"" + info.hr_name + "_type:value\"")
				}

				matched_bits := []string{to_bits[0], to_bits[1]}
				var err error
				to_mount, matched_bits[0], err = fuzzy_reverse_lookup(mounts, to_bits[0], "mount")
				if err != nil {
					return err
				}

				to = to_bits[1]
				mount_matched = matched_bits[0]
			}

			var to_value interface{}
			if to == "empty" && (info.data_type&DT_INT != 0) {
				// Handle "empty" special case first.
				// This is a bit of a crock.  We exploit the fact that there are no string fields that can be empty
				// (if there were, we would have to have some way to know if that "empty" is the string "empty" or the magic word "empty")
				if !info.can_be_empty {
					return errors.New(info.hr_name + " can't be empty")
				}
				to_value = nil
				matched += to
			} else if len(info.trans_str) > 0 {
				// lookup map is "backwards" from the setting PoV
				k, m, err := fuzzy_reverse_lookup(info.trans_str, to, info.hr_name)
				if err != nil {
					return err
				}
				to_value = k
				matched += m
			} else if info.data_type&DT_STRING != 0 {
				// No lookup available, but data is a string, so use it directly
				to_value = to
				matched += to
			} else if info.data_type&DT_INT != 0 {
				// Try lookup first...
				lookup_match := false
				if info.trans_int != nil {
					// Another backwards map
					k, m, err := fuzzy_reverse_lookup(info.trans_int(savedata.Game()), to, info.hr_name)
					if err != nil {
						if (info.data_type & DT_ALLOW_NUM) == 0 {
							return err
						}

					}
					to_value = k
					matched += m
					lookup_match = true
				}

				if !lookup_match {
					// No match from lookup - but raw numbers are allowed, so see if we have a number
					var int_value int
					if (info.data_type & DT_FIXED) != 0 {
						float_value, err := strconv.ParseFloat(to, 32)
						if err != nil {
							return err
						}
						int_value = int(float_value * 256.0)
						if int_value < info.int_min || int_value > info.int_max {
							return errors.New(fmt.Sprintf("%v number must be between %v and %v", info.hr_name, float32(info.int_min)/256.0, float32(info.int_max)/256.0))
						}
					} else {
						// not fixed, must be regular int
						int_value, err = strconv.Atoi(to)
						if err != nil {
							return err
						}
						if int_value < info.int_min || int_value > info.int_max {
							return errors.New(info.hr_name + " number must be between " + strconv.Itoa(info.int_min) + " and " + strconv.Itoa(info.int_max))
						}
					}
					to_value = int_value
					matched += to
				}
			} else {
				return errors.New(fmt.Sprintf("Internal privedit error: ettables[%v] failed to specify an action ", what))
			}

			name := info.hr_name
			if mount_matched != "" {
				name += ":" + mount_matched
			}
			setargses = append(setargses, setArgs{what, to_value, to_mount, name, matched})
		}
		return nil
	}()
	return
}

func load(full_filename string) (*types.Savedata, error) {
	reader, err := os.Open(full_filename)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	return types.Read_savedata(reader)
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
	err = encoder.Decode(&filename)
	if err != nil {
		return "", nil, err

	}
	savedata := types.Savedata{}
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
			return zero[K](), "", errors.New(fmt.Sprint("Ambiguous argument:", to, " could be anything from {", strings.Join(names, ", "), "}"))
		}

		return matches[0], names[0], nil
	}

	return zero[K](), "", errors.New(to + " could not be matched to a valid value for " + what)
}

// get gets something and returns it
// what: the thing to be got
// savedata: processed savefile data
// returns a savefile-friendly value e.g. 7 not "Tachyon Cannon"; how to convert this to something useful is up to the caller
func get(what etype, savedata *types.Savedata) (interface{}, error) {
	g := ettables[what]
	mounted := g.data_type&(DT_HASMOUNT|DT_ADDMOUNT) != 0

	bytes := []uint8{}
	switch g.chunk_type {
	case CT_STRING:
		return savedata.Strings[g.offset].Value, nil

	case CT_FORM:
		record := savedata.Forms[g.offset].Get(g.record...)
		if record == nil {
			// Not actually an error; sometimes equipment just isn't installed
			if mounted {
				// Simulate the "there's nothing there" return value form get_at_mount.
				// the problem here is that while a nil map will act like an empty map, a nil interface
				// does not act like an interface containing a nim map.
				return map[int]interface{}{}, nil
			}
			return nil, nil
		}
		end := g.end
		if end < 0 {
			end += (len(record.Data) + 1) // +1 because negative indices have to start at -1, not 0
		}
		bytes = record.Data[g.start:end]

	case CT_BLOB:
		bytes = savedata.Blobs[g.offset][g.start:g.end]
	}

	if mounted {
		return get_at_mounts(what, bytes, savedata)
	}

	if g.data_type&DT_STRING != 0 {
		return string(bytes), nil
	}

	n, err := read_int(bytes)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// set sets something
// Exactly how to set something is encoded in the "ettables" data
// what: the thing to be set
// to: the value to set it to.  This is a savefile-friendly value e.g. 7 not "Tachyon Cannon".
//
//	This may be nil; that means "equipment not present".
//
// savedata: processed savefile data
//
// set does not check for argument inconsistencies (e.g to==nil but ettables[what].can_be_empty==false); that should have happened already
// set does not check for game-crash-causing holistic savefile inconsistencies; that happens later in sanity_fix
func set(what etype, to interface{}, savedata *types.Savedata, log Logger) error {
	info := ettables[what]
	should_be_empty := (to == nil)

	if info.chunk_type == CT_STRING {
		// Special case: StringChunk contains a proper string (not a byte array)
		// Also, strings can't be empty since "" is not a special value.
		cl := savedata.Chunk(info.offset).Chunk_length()
		to_str := to.(string)
		if len(to_str)+1 > cl { //+1 for the null terminator
			// TODO: in "I know what I'm doing" mode, this should just be a warning
			return errors.New(fmt.Sprintf("Failed - new %v has %v characters; max length is %v", what, len(to_str), cl))
		}
		savedata.Strings[info.offset].Value = to_str
		return nil
	}

	// Find the target (byte array to write into)
	var target []byte
	switch info.chunk_type {
	case CT_BLOB:
		target = savedata.Blobs[info.offset]
	case CT_FORM:
		record := savedata.Forms[info.offset].Get(info.record...)
		var err error
		if record == nil && !should_be_empty {
			record, err = add_new_record(savedata, info.offset, info.record)
			if err != nil {
				return err
			}
		}
		if record != nil && should_be_empty {
			savedata.Forms[info.offset].Delete_record(info.record...)
			return nil
		}
		if record == nil && should_be_empty {
			log.Logln(info.hr_name, "is already empty, so... done, I guess?")
			return nil
		}

		target = record.Data
	}

	// "Write" into the target...
	switch info.data_type & (DT_INT | DT_STRING) {
	case DT_INT:
		write_int(to.(int), info.end-info.start, target[info.start:info.end])
	case DT_STRING:
		end := info.end
		if end < 0 {
			end += (len(target) + 1) // +1 because negative indices have to start at -1, not 0
		}
		target = append(target[:info.start], append([]byte(to.(string)), target[end:]...)...)
	}

	// ...except that maybe we didn't really write into actual data so write "target" back onto where it should be
	// (append may have created a completely new byte aray, and we can't get round this using pointer-to-pointer
	//  because values in maps aren't addressable.  Et tu, Go?)
	switch info.chunk_type {
	case CT_BLOB:
		savedata.Blobs[info.offset] = target
	case CT_FORM:
		savedata.Forms[info.offset].Get(info.record...).Data = target
	}

	return nil
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

func safe_lookup_fixed(from map[int]string, with int) string {
	strvalue := fmt.Sprintf("%v", float32(with)/256.0)
	if from == nil {
		return strvalue
	}
	out, ok := from[with]
	if !ok {
		return strvalue
	}
	return out
}

// get_at_mounts is an alternative version of get for  "mountable" things
// "mountable" includes more than actually-mounted equipment; it covers anything where there is an extra type value
// and there can be at most one thing with a given type.  e.g reputation values are "mounted" at a faction and cargo
// quantities are "mounted" at a cargo type.
//
// This function gets *all* the data for a given type, including where it is mounted (so no mount argument is needed)
// returned map keys are mount IDs; map values are what is mounted there
func get_at_mounts(what etype, data []byte, savedata *types.Savedata) (map[int]interface{}, error) {
	out := map[int]interface{}{}
	minfo := mount_infos[what]
	cl := minfo.chunk_length
	for i := 0; i < len(data); i += cl {
		thing, err := read_int(data[i+minfo.equipment_offset : i+minfo.equipment_offset+minfo.equipment_length])
		if err != nil {
			return nil, err
		}

		mount := 0
		switch ettables[what].data_type & (DT_HASMOUNT | DT_ADDMOUNT) {
		case DT_HASMOUNT:
			mount = int(data[i+minfo.mount_offset])
		case DT_ADDMOUNT:
			mount = i / cl
		}
		out[mount] = thing
	}
	return out, nil
}

// set_at_mount is an alternate version of set for "mountable" things
func set_at_mount(what etype, to interface{}, to_mount int, savedata *types.Savedata, log Logger) error {
	info := ettables[what]
	if info.data_type&(DT_INT|DT_STRING) != DT_INT {
		return errors.New("Internal privedit error: atttempt to mount a string type")
	}
	to_thing := 0
	should_be_empty := (to == nil)
	if !should_be_empty {
		to_thing = to.(int)
	}

	record := savedata.Forms[info.offset].Get(info.record...)
	if record == nil {
		var err error
		record, err = add_new_record(savedata, info.offset, info.record)
		if err != nil {
			return err
		}
	}
	target := &record.Data
	minfo := mount_infos[what]
	cl := minfo.chunk_length

	// DT_ADDMOUNT is the easy case - everything always exists, and simply adding
	// the mount (multiplied by chunk length) tells us where we need to write
	if info.data_type&DT_ADDMOUNT != 0 {
		err := write_int(to_thing, cl, (*target)[to_mount*cl:to_mount*cl+cl])
		if err != nil {
			return err
		}
		return nil
	}

	// DT_HASMOUNT case...
	equipment := sniff1(ettables[what].trans_int)(savedata.Game())
	eq_new_str := safe_lookup(equipment, to_thing)
	mount_str := safe_lookup(mount_infos[what].mounts, to_mount)

	for i := 0; i < len(*target); i += cl {
		thing, err := read_int((*target)[i+minfo.equipment_offset : i+minfo.equipment_offset+minfo.equipment_length])
		if err != nil {
			return err
		}
		eq_old_str := safe_lookup(equipment, thing)

		mount := int((*target)[i+minfo.mount_offset])
		if mount == to_mount {
			// equipment exists...
			if should_be_empty {
				// ...but should not
				log.Logln("Destroying existing", eq_old_str, "at", mount_str)
				(*target) = append((*target)[:i], (*target)[i+cl:]...)
				return nil
			}
			// ...and should exist
			log.Logln("Transmogrifying existing", eq_old_str, "at", mount_str, "into a", eq_new_str)
			err := write_int(to_thing, minfo.equipment_length, (*target)[i+minfo.equipment_offset:])
			if err != nil {
				return err
			}
			return nil
		}
	}

	if should_be_empty {
		// equipment does not exist and doesn't need to
		log.Logln(mount_str, "is already empty, so... done, I guess?")
		return nil
	}

	// equipment does not exist but needs to
	log.Logln("Adding new", eq_new_str, "at", mount_str)
	new_data := make([]byte, cl)
	write_int(to_thing, minfo.equipment_length, new_data[minfo.equipment_offset:])
	new_data[minfo.mount_offset] = byte(to_mount)
	*target = append(*target, new_data...)
	return nil
}

// sanity_fix attempts to fix inconsistencies in savedata - but only the ones that would result in things the player doesn't like, such as immediate game crashes.
// Since the game doesn't care about silly little things like quad-Steltek-gun Centurions with level 5 engines and a 3rd launcher in the nonexistent rear turret, we don't either.
//
// Multiple steltek guns are in fact a partial exception - this is so much fun that we allow it, even though ships so equipped can't be traded.
func sanity_fix(savedata *types.Savedata, log Logger) {
	// Turret mounts:   1: Rear, 2:top, 3:bottom
	// Gun mounts: 		1: Left outer, 2: Left, 3: Right, 4: Right outer, 5: Turret 1a, 7: turret 1b, 8: turret 2a, 10 turret 2b
	// Only the Centurion has outer mounts.
	// Launcher mounts: 0: Centre, 1: Left (not Centurion), 2: Left (Centurion), 3: Right (Centurion), 4: Right (not Centurion), 6: turret 1, 9: turret 2
	type fixers struct { //key is bad slot, value is alternative good slot
		fix_turrets   map[byte]int
		fix_guns      map[byte]int
		fix_launchers map[byte]int
	}
	// We try to "fix" bad equipment by moving it to a corresponding allowed slot.
	// However, since ships don't even have the same numbers of mounts, weapons
	// must sometimes be thrown away.
	// TODO: add enums to tables.go so we don't have a ridiculous pile of magic numbers here.
	mounts := map[uint8]fixers{
		tables.SHIP_TARSUS:    {map[byte]int{1: -1, 2: -1, 3: -1}, map[byte]int{1: 2, 4: 3, 5: -1, 7: -1, 8: -1, 10: -1}, map[byte]int{0: -1, 2: 1, 3: 4, 6: -1, 9: -1}},
		tables.SHIP_ORION:     {map[byte]int{2: -1, 3: -1}, map[byte]int{1: 2, 4: 3, 8: -1, 10: -1}, map[byte]int{1: -1, 2: -1, 3: -1, 4: -1, 9: -1}},
		tables.SHIP_CENTURION: {map[byte]int{2: -1, 3: -1}, map[byte]int{8: -1, 10: -1}, map[byte]int{0: -1, 1: 2, 4: 3, 9: -1}},
		tables.SHIP_GALAXY:    {map[byte]int{1: -1}, map[byte]int{1: 2, 4: 3}, map[byte]int{0: -1, 1: 2, 4: 3}},
	}

	fix_record := func(weapon etype, fixer map[byte]int) {
		info := ettables[weapon]
		hr_weapon := info.hr_name
		record := savedata.Forms[info.offset].Get(info.record...)
		if record == nil {
			// Not an error, sometimes records are empty if there's no equipment
			return
		}

		minfo := mount_infos[weapon]
		cl := minfo.chunk_length
		data := record.Data
		new_data := [][]byte{}
		new_data_by_mount := map[int]int{}
		// go through "data", read each chunk, write into "new_data" (modified, if necessary)
		// The reason for doing it this way is that we don't want to change the order (particularly in the case where we don't need to change anything)
		// Although the game doesn't care if we randomize the order, tests are much easier to write if file output is deterministic and
		// as order-preserving as it can be.
		for i := 0; i < len(data); i += cl {
			old_mount := data[i+minfo.mount_offset]
			new_mount, bad := fixer[old_mount]
			if !bad {
				// no fixed mount - weapon is allowed to exist where it is
				new_mount = int(old_mount)
			}

			nd_i, occupied := new_data_by_mount[new_mount]
			if new_mount == -1 || (occupied && data[i+minfo.equipment_offset] < new_data[nd_i][minfo.equipment_offset]) {
				// If new mount is occupied then weapon with the larger ID wins
				// This is to avoid putting a good  savefile into an unwinnable state by throwing away steltek guns (which have the highest IDs)
				// (If we're not called in a gun context, then it doesn't matter what we keep or throw away)
				log.Logln("Sanity fix:", hr_weapon, "from mount", old_mount, "thrown away")
			} else {
				if occupied {
					new_data[nd_i] = data[i : i+cl]
				} else {
					new_data = append(new_data, data[i:i+cl])
					nd_i = len(new_data) - 1
				}
				new_data[nd_i][minfo.mount_offset] = byte(new_mount)
				new_data_by_mount[new_mount] = nd_i

				if new_mount != int(old_mount) {
					if occupied {
						log.Logln("Sanity fix:", hr_weapon, "from mount", new_mount, "thrown away")
					}
					log.Logln("Sanity fix:", hr_weapon, "moved from mount", old_mount, "to mount", new_mount)
				}
			}
		}

		record.Data = []byte{}
		for _, subdata := range new_data {
			record.Data = append(record.Data, subdata...)
		}
	}

	ship := savedata.Blobs[types.OFFSET_SHIP][0]
	fix_record(ET_TURRET, mounts[ship].fix_turrets)
	// We do not add turrets merely because existing equipment demands it, because the game doesn't seem to care.
	fix_record(ET_GUN, mounts[ship].fix_guns)
	fix_record(ET_LAUNCHER, mounts[ship].fix_launchers)

	// Engine damage...
	// It looks like engine info is a list of engine subcomponents, and engine damage info is a list of damage-per-subcomponent values.
	// Consequently, a change in engine could result in a change of engine damage info length.  Not doing this seems to cause
	// the game to read off the end of REAL-FITE-ENER-DAMG to find damage values, resulting in nonsensical data and ludicrous repair fees.
	//
	// Just to make things more interesting, REAL-FITE-ENER-DAMG length is not preserved by launch-landing.  We use the longer length here,
	// which is the immediately-after-buying length, mostly because we understand how to calculate it.
	// TODO: understand how to calculate the smaller value, only update if necessary, log iff update happened.
	engine_subcomponents := (len(savedata.Forms[types.OFFSET_REAL].Get("FITE", "ENER", "INFO").Data) - len("ENERGY") - 2) / 2
	savedata.Forms[types.OFFSET_REAL].Get("FITE", "ENER", "DAMG").Data = make([]byte, engine_subcomponents*14)

	// Shield damage.
	// this always has fixed length, but if we added a shield, we must add this
	has_shield := savedata.Forms[types.OFFSET_REAL].Get("FITE", "SHLD", "INFO") != nil
	has_shield_damg := savedata.Forms[types.OFFSET_REAL].Get("FITE", "SHLD", "DAMG") != nil
	if has_shield && !has_shield_damg {
		log.Logln("Adding 0-damage shield damage record")
		add_new_record(savedata, types.OFFSET_REAL, []string{"FITE", "SHLD", "DAMG"})
	}
	if !has_shield && has_shield_damg {
		// Get rid of the shield damage then.
		// This is probably unnecessary but good houseguests don't shit on the floor
		log.Logln("Removing shield damage record")
		savedata.Forms[types.OFFSET_REAL].Delete_record("FITE", "SHLD", "DAMG")
	}

	// Launcher order
	// front-mounted launchers must appear before turret-mounted launchers (or the game crashes when user presses 'W')
	launchers := savedata.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH")
	if launchers != nil {
		cl := mount_infos[ET_LAUNCHER].chunk_length
		mo := mount_infos[ET_LAUNCHER].mount_offset
		d := launchers.Data
		// Fix the problem by sorting (because all front mounts have samller IDs than all non-front mounts)
		// Bubblesort, but n will never be larger than 4.  Ugh.
		for i1 := 0; i1 < len(d)-cl; i1 += cl {
			for i2 := i1 + cl; i2 < len(d); i2 += cl {
				if d[i1+mo] > d[i2+mo] {
					log.Logln("Sanity fix: reordering launchers")
					d[i1+mo], d[i2+mo] = d[i2+mo], d[i1+mo]
				}
			}
		}
	}

	// Torpedo check
	// The game doesn't like it if you have torpedos but no torpedo launcher
	// (having missiles but no missile launcher is of course completely fine)
	// This is probably because torpedos are split evenly between each launcher, which results in division by 0.
	has_torp_launcher := false
	launchers = savedata.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "LNCH")
	if launchers != nil {
		d := launchers.Data
		cl := mount_infos[ET_LAUNCHER].chunk_length
		eo := mount_infos[ET_LAUNCHER].equipment_offset
		for i := 0; i < len(d); i += cl {
			if d[i+eo] == 51 {
				has_torp_launcher = true
				break
			}
		}
	}
	if !has_torp_launcher {
		missiles := savedata.Forms[types.OFFSET_REAL].Get("FITE", "WEAP", "MISL")
		if missiles != nil {
			d := missiles.Data
			cl := mount_infos[ET_MISSILE].chunk_length
			eo := mount_infos[ET_MISSILE].equipment_offset
			// Iterating backwards ensures that deletion doesn't screw things up
			for i := len(d) - cl; i >= 0; i -= cl {
				if d[i+eo] == 1 {
					log.Logln("Sanity fix: destroying torpedo stack at position", i/cl, "due to lack of launchers")
					missiles.Data = append(d[:i], d[i+cl:]...)
				}
			}
		}
	}
}

// read_int and write_int always read/write in little-endian.
// Since the file data is always little-endian (whereas metadata is always big-endian), this is what we want.

func read_int(data []byte) (int, error) {
	n := 0
	switch len(data) {
	case 0:
		n = 0
	case 1:
		n = int(data[0])
	case 2:
		n, _ = readers.Read_int16(bytes.NewReader(data))
	case 4:
		n, _ = readers.Read_int_le(bytes.NewReader(data))
	default:
		return 0, errors.New("Internal privedit error: unexpected byte length for field")
	}

	return n, nil
}

func write_int(n int, length int, target []byte) error {
	if length == 0 && n != 0 {
		// validate int0
		return errors.New("Internal privedit error: attempt to write non-zero number to empty")
	}
	switch length {
	case 0, 1, 2, 4: // OK
	default:
		return errors.New("Internal privedit error: unexpected byte length for field")
	}

	for i := 0; i < length; i += 1 {
		target[i] = uint8((n >> (8 * i)) & 0xFF)
	}
	return nil
}
