package main

import (
	"fmt"
	"os"

	"privdump/readers"
)

//rip.go
//
// This is useful only for ripping data out of PRIV.TRE (whose full filename must be the first and only argument)
// currently, it extracts the indices of systems and bases.
//
// (If you have the GOG version, "game.gog" is an iso containing this file (and others))

func main() {

	filename := os.Args[1]

	f, err := os.Open(filename)
	fmt.Println(err)

	// TODO: this is really dirty...
	// We just hard-code the location of the form in priv.tre
	// Looking for the form would probably be better
	_, err = f.Seek(0x6C5644, 0)
	fmt.Println(err)

	//systems
	//0xB82
	bytes_ := [0xB82 + 8]byte{}
	bytes := bytes_[:]
	f.Read(bytes)

	cur := 0
	form, _ := readers.Read_form(bytes, &cur)

	parents := map[uint8]string{}
	name := ""
	for _, quad := range form.Records {
		for _, sf := range quad.Forms {
			for _, r2 := range sf.Records {
				//if r2.name=="INFO"{
				//	fmt.Println(r2.data[:4], string(r2.data[4:]), "Quadrant:")
				//}
				for _, syst := range r2.Forms {
					//fmt.Println("   ", syst.name)
					for _, r3 := range syst.Records {
						if r3.Name == "INFO" {
							//fmt.Println(r3.data[1:5])
							name = string(r3.Data[5 : len(r3.Data)-1])
							fmt.Println(fmt.Sprintf("%v: \"%v\",", r3.Data[0], name))
						}
						if r3.Name == "BASE" {
							fmt.Println("Bases:", r3.Data)

							for _, d := range r3.Data {
								parents[d] = name
							}
						}
					}
				}
			}
		}
		fmt.Println()
	}

	// TODO: Again, ridiculosuly dirty hard-coding

	//Bases
	_, err = f.Seek(0x6C61CE, 0)
	fmt.Println(err)

	//0x482
	bytes__ := [0x482 + 8]byte{}
	bytes = bytes__[:]
	f.Read(bytes)

	fmt.Println(bytes[:16])

	cur = 0
	bases, err := readers.Read_form(bytes, &cur)
	fmt.Println(err)

	type_ := []string{"", "Pleasure", "Refinery", "Mining", "Agricutural", "Pirate", "Special"}
	for _, info := range bases.Records {
		if info.Name == "INFO" {
			fmt.Println(info.Data[:1], string(info.Data[2:]), "-", type_[info.Data[1]], fmt.Sprintf("(%v)", parents[info.Data[0]]))
			//fmt.Println(fmt.Sprintf("%v: \"%v (%v)\",", info.Data[0], string(info.Data[2:]), parents[info.Data[0]]))
		} else {
			fmt.Println(info.Name, info.Data)
		}
	}
}
