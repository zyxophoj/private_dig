package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"gopkg.in/ini.v1"

	"privdump/readers"
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

	// Now for the really evil part.  Detect FORMs that are lying about their lengths!
	for i := range chunks {
		if len(chunks[i]) < 12 {
			// too short to be a FORM
			continue
		}
		r := bytes.NewReader(chunks[i])
		_, err := readers.Read_fixed_string("FORM", r)
		if err != nil {
			// not a form
			continue
		}

		length, _ := readers.Read_int_be(r)
		length += 8 // because read length does not include the stuff we already read
		for length > len(chunks[i]) {
			// F is for footer?  In any case, 'F' the form until it is no longer lying about its length
			chunks[i] = append(append([]byte{}, chunks[i]...), byte('F')) // Beware of append side-effects!  UGH!
		}
	}

	return chunks
}

// savefiles_equal "loosely" compares savefiles
// We would like to do a byte-for-byte compare, but that wouldn't work - because real privateer savefiles
// may contain forms that lie about their lengths.
// Savedata.Write does not commit this crime against humanity, which means load-saving will not produce
// an identical file - so we fix that bullshit, then compare as byte-for-byte as we can.
func savefiles_equal(file1 []byte, file2 []byte) (bool, error) {
	chunks1 := basic_chunk_split(file1)
	chunks2 := basic_chunk_split(file2)

	for i := 0; i < len(chunks1); i += 1 {
		if len(chunks1[i]) != len(chunks2[i]) {
			return false, fmt.Errorf("Length mismatch: %v != %v", len(chunks1[i]), len(chunks2[i]))
		}
		for j := range chunks1[i] {
			if chunks1[i][j] != chunks2[i][j] {
				return false, fmt.Errorf("Data mismatch in chunk %v at %v (out of %v), (%v != %v)", i, j, len(chunks1[i]), int(chunks1[i][j]), int(chunks2[i][j]))
			}
		}

	}
	return true, nil
}

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
		// Something has clearly gone wrong her,e and we want to avoid vacuous success
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

		// Sadly, data length isn't conserved because real files contain forms that lie about their lengths
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
