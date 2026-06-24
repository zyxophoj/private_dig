package readers

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"slices"
)

func Read_fixed(r io.Reader, size int) ([]byte, error) {
	into := make([]byte, size)
	n, err := r.Read(into)
	if err != nil {
		return nil, err
	}
	if n != size {
		return nil, errors.New(fmt.Sprintf("Failed to read ^b bytes (got only %v)", size, n))
	}

	return into, nil
}

// Advance advances a reader forwards
// It's a forward-only seek for something that doesn't seek.
func Advance(r io.Reader, size int) error {
	_, err := Read_fixed(r, size)
	return err
}

func Read_string(r io.Reader) (string, int, error) {
	//reads null-terminated string,  Advances the reader past the terminating null.
	one := []byte{0}
	out := []byte{}

	for {
		n, err := r.Read(one)
		if err != nil {
			return "", 0, err
		}
		if n != 1 {
			return "", 0, errors.New("failed to read")
		}

		if one[0] == 0 {
			return string(out), len(out) + 1, nil
		}

		out = append(out, one[0])
	}
}

func Read_fixed_string(target string, r io.Reader) (int, error) {
	target_buf := []byte(target)
	read_buf, err := Read_fixed(r, len(target_buf))

	if err != nil {
		return 0, err
	}

	if !slices.Equal(read_buf, target_buf) {
		return 0, errors.New("Could not find string " + target + "(got " + string(read_buf) + ")")
	}

	return len(read_buf), nil
}

func Read_int_be(r io.Reader) (int, error) {
	var out int32
	err := binary.Read(r, binary.BigEndian, &out)
	return int(out), err
}

func Read_int_le(r io.Reader) (int, error) {
	var out int32
	err := binary.Read(r, binary.LittleEndian, &out)
	return int(out), err
}

func Read_int16(r io.Reader) (int, error) {
	var out int16
	err := binary.Read(r, binary.LittleEndian, &out)
	return int(out), err
}

func Read_uint16(r io.Reader) (int, error) {
	var out uint16
	err := binary.Read(r, binary.LittleEndian, &out)
	return int(out), err
}
