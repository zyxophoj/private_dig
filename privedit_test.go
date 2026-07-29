package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"gopkg.in/ini.v1"

	"privdump/readers"
	"privdump/tables"
	"privdump/types"
)

import "fmt"

// TODO: unduplicate any test-utility functions that appear both here and in test_ach

var test_dir = "ach_test"

// real_filename constructs an actual filename from a string in the ini file.
// These strings typically don't have extensions, so guess the extension based on
// RF-ness.
// However, if the string does have an extension, use it
func real_filename(file string, RF bool) string {
	ext := ".SAV"
	if RF {
		ext = ".PRS"
	}
	filename := test_dir + "/" + strings.ToUpper(strings.TrimSpace(file))
	if !strings.Contains(file, ".") {
		filename += ext
	} else {
		if RF && !strings.HasSuffix(filename, ".PRS") {
			// This would be a bug in the ini file
			fmt.Println("Error: Non-RF file read in RF mode")
		}
	}

	return filename
}

// min exists because math.Min only works for floats
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func basic_chunk_split(file []byte) [][]byte {
	// Read chunks out of the file
	chunks := [][]byte{}
	_nchunks, _ := readers.Read_int16(bytes.NewReader(file[4:]))
	nchunks := _nchunks/4 - 1
	bounds := []int{}
	for i := 4; i < 4+4*nchunks; i += 4 {
		chunk_start, _ := readers.Read_int16(bytes.NewReader(file[i:]))
		bounds = append(bounds, chunk_start)
	}
	bounds = append(bounds, len(file))
	for i := 0; i < nchunks; i += 1 {
		chunks = append(chunks, file[bounds[i]:bounds[i+1]])
	}

	return chunks
}

// savefiles_equal "loosely" compares savefiles
// We would like to do a byte-for-byte compare, but that wouldn't work - because real privateer savefiles
// may contain forms that lie about their lengths.
// Savedata.Write does not commit this crime against humanity, which means load-saving will not produce
// an identical file - so we compare as close to byte-for-byte as we can get.
func savefiles_equal(file1 []byte, file2 []byte) (bool, error) {
	chunks1 := basic_chunk_split(file1)
	chunks2 := basic_chunk_split(file2)

	for i := 0; i < len(chunks1); i += 1 {
		for j := range min(len(chunks1[i]), len(chunks2[i])) {
			if chunks1[i][j] != chunks2[i][j] {
				return false, fmt.Errorf("Data mismatch in chunk %v at %v (out of %v), (%v != %v)", i, j, len(chunks1[i]), int(chunks1[i][j]), int(chunks2[i][j]))
			}
		}

	}
	return true, nil
}

// Mock up something that looks like a Logger
type BogoLogger struct{}

func (bl *BogoLogger) Logln(a ...any)                {}
func (bl *BogoLogger) Logfn(str string, strs ...any) {}

var log *BogoLogger

// The most basic test - can a file survive load-stash-retrieve-save (equivalent to privedit load and privedit save)?
func Test_LoadStashRetrieveSave(t *testing.T) {

	// Get filenames
	filenames := map[string]bool{}
	inifile, err := ini.Load(test_dir + "/files.ini")
	if err != nil {
		t.Errorf("cant' even read ini file: %v", err)
	}
	for _, s := range inifile.Sections() {
		if s.Name() == "DEFAULT" {
			continue
		}
		is_rf := strings.HasPrefix(s.Name(), "AID_RF_")
		for _, key := range []string{"yes", "no", "multi"} {
			for _, filename := range strings.Split(s.Key(key).String(), ",") {
				if filename != "" {
					filenames[real_filename(filename, is_rf)] = true
				}
			}
		}
	}
	if len(filenames) == 0 {
		// Something has clearly gone wrong here and we want to avoid vacuous success
		t.Error("No filenames read!")
	}

	error_count := 0
	success_count := 0
	for filename := range filenames {
		file_bytes, err := os.ReadFile(filename)
		if err != nil {
			t.Logf("failed to load file %v, %v", filename, err)
			error_count++
			continue
		}
		sd, err := types.Read_savedata(bytes.NewReader(file_bytes))
		if err != nil {
			t.Logf("failed to read file %v, %v", filename, err)
			error_count++
			continue
		}
		stash(filename, sd)
		filename2, sd2, err := retrieve()
		if err != nil {
			t.Logf("failed to retrieve file %v, %v", filename, err)
			error_count++
			continue
		}
		if filename2 != filename {
			t.Logf("Can't even get filenames right! (%v -> %v)", filename, filename2)
			error_count++
			continue
		}
		out_buf := &bytes.Buffer{}
		sd2.Write(out_buf)

		// Sadly, data length isn't conserved because real files contain dishonest forms
		/*if len(out_buf.Bytes()) != len(file_bytes){
			t.Logf("Data length not conserved by load->stash->retrieve->save (%v -> %v) (%v)", len(file_bytes), len(out_buf.Bytes()), filename)
			error_count++
			continue
		}*/
		equal, err := savefiles_equal(file_bytes, out_buf.Bytes())
		if err != nil {
			t.Logf("failed to compare file with itself: %v, %v", filename, err)
			error_count++
			continue
		}
		if !equal {
			t.Logf("Data Mangled by load->stash->retrieve->save (%v)", filename)
			error_count++
			continue
		}

		success_count++
	}

	if error_count > 0 {
		t.Errorf("Errors! (%v errors, %v successes)", error_count, success_count)
	}
}

// test the "set" function - at least in the non-mountable cases
// (this also depends on load() and get())
func Test_SetSimple(t *testing.T) {
	// NEW.SAV was created by immediately saving from a new game
	filename := real_filename("NEW", false)
	sd, err := load(filename)
	if err != nil {
		t.Errorf("Failed to load file %v - %v", filename, err)
	}

	tests := []struct {
		what etype
		from interface{}
		to   interface{}
	}{
		{ET_SHIP, int(tables.SHIP_TARSUS), int(tables.SHIP_CENTURION)},
		{ET_LOCATION, int(tables.BASE_ACHILLES), int(tables.BASE_NEW_DETROIT)},
		{ET_CREDITS, 2000, 12345678},
		{ET_SHIELD, tables.SHIELD_BASE_0 + 1, tables.SHIELD_BASE_0 + 5},
		//{ET_ENGINE,   0,5},  TODO!!
		{ET_NAME, "test", "Blair"},
		{ET_CALLSIGN, "test", "Maverick"},
	}

	for _, test := range tests {
		old, _ := get(test.what, sd)
		if old != test.from {
			t.Errorf("Starting %v not as expected (got %v, expected %v)", ettables[test.what].hr_name, old, test.from)
		}

		set(test.what, test.to, sd, nil)

		new_, _ := get(test.what, sd)
		if new_ != test.to {
			t.Errorf("Modified %v not as expected (got %v, expected %v)", ettables[test.what].hr_name, new_, test.to)
		}
	}
}

// test the set_at_mount function
// (this also depends on load() and get())
func Test_SetAtMount(t *testing.T) {
	filename := real_filename("NEW", false)
	sd, err := load(filename)
	if err != nil {
		t.Errorf("Failed to load file %v - %v", filename, err)
	}

	// Some of these tests leave the ship in an illegal state.
	//We don't care because we're not testing sanity_fix here.
	tests := []struct {
		what  etype
		from  interface{}
		to    interface{}
		where int
	}{
		// the DT_HASMOUNT types have a second "nil" line here that tests removing the equipment.
		{ET_GUN, tables.GUN_LASER, tables.GUN_TACHYON_CANNON, tables.GM_LEFT},
		{ET_GUN, tables.GUN_TACHYON_CANNON, nil, tables.GM_LEFT},
		{ET_LAUNCHER, tables.LAUNCH_MISSILE, tables.LAUNCH_TORPEDO, tables.LM_RIGHT},
		{ET_LAUNCHER, tables.LAUNCH_TORPEDO, nil, tables.LM_RIGHT},
		{ET_MISSILE, 5, 100, tables.MIS_DUMBFIRE},
		{ET_MISSILE, 100, nil, tables.MIS_DUMBFIRE},
		{ET_TURRET, nil, 0, tables.TM_REAR}, // Here, nil is "not present" and 0 is "present".  I am sorry.
		{ET_TURRET, 0, nil, tables.TM_REAR}, // Here, nil is "not present" and 0 is "present".  I am sorry.
		{ET_REPUTATION, 0, 1234, tables.FACTION_MERCHANTS},
		{ET_KILLS, 0, 2345, tables.FACTION_KILRATHI},
	}

	for _, test := range tests {
		old_map, _ := get(test.what, sd)
		fmt.Println(test)
		fmt.Println(old_map)
		old := old_map.(map[int]interface{})[test.where]
		if old != test.from {
			t.Errorf("Starting %v not as expected (got %v, expected %v)", ettables[test.what].hr_name, old, test.from)
		}
		set_at_mount(test.what, test.to, test.where, sd, log)
		new_map, _ := get(test.what, sd)
		new_ := new_map.(map[int]interface{})[test.where]
		if new_ != test.to {
			t.Errorf("Modified %v not as expected (got %v, expected %v)", ettables[test.what].hr_name, new_, test.to)
		}
	}
}


// TODO: test sanity_fix
// TODO: test command parsing
