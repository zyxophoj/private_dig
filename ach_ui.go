package main

// Graphical front end for Privateer achievements

import (
	"errors"
	"fmt"
	"image/color"
	"strconv"
	"time"
	"unicode"

	"gopkg.in/ini.v1"

	"golang.org/x/image/font/basicfont"

	"github.com/gopxl/pixel/v2"
	"github.com/gopxl/pixel/v2/backends/opengl"
	"github.com/gopxl/pixel/v2/ext/text"

	"privdump/priv_ach"
	"privdump/utils"
)

func solid_rect_sprite(rect pixel.Rect, colour color.RGBA) *pixel.Sprite {
	pd := pixel.MakePictureData(rect)
	for i := range pd.Pix {
		pd.Pix[i] = colour
	}
	return pixel.NewSprite(pd, pd.Bounds())
}

// color_from_string converts an ini file colour string (e.g. "R255g128b0") into a color.RGBA
// the alpha part of RGBA just gets set to 0xff (full opacity) if omitted
// (r, g, and b get set to 0 if omitted, which means "g42" or even an empty string is technically a valid color string, but please don't do that)
func color_from_string(str string) (color.RGBA, error) {
	out := color.RGBA{0, 0, 0, 0xFF}

	name := rune(0)
	numstr := ""
	for _, r := range str + "!" { // +"!" is an evil way to make sure the final colour index gets processed.
		if unicode.IsDigit(r) {
			numstr += string(r)
		} else {
			if name != 0 {
				number, _ := strconv.Atoi(numstr)
				if number > 255 {
					number = 255
				}
				switch name {
				case 'r', 'R':
					out.R = uint8(number)
				case 'g', 'G':
					out.G = uint8(number)
				case 'b', 'B':
					out.B = uint8(number)
				case 'a', 'A':
					out.A = uint8(number)
				default:
					return out, errors.New("Unexpected colour index (not 'r', 'b' or 'g'): " + string(name))
				}
				numstr = ""
			}
			name = r
		}
	}
	return out, nil
}

// clean cleans a slice (i.e. throws away anything that is inaccessible but still in the underlying array, allowing GC to happen)
// This does copy the part of the slice that should remain.
func clean[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

func main() {
	// OpenGL must have the main thread
	opengl.Run(run)
}

func run() {
	// superconstants
	const TEXT_LINES = 3
	const LINE_HEIGHT = 13 // because hard-coded basicfont.Face7x13.

	const TEXT_HEIGHT = (TEXT_LINES + 1) * LINE_HEIGHT // 3 lines of text plus a gap
	const TEXT_H_OFFSET = LINE_HEIGHT                  // Pixel2 bases text drawing from the bottom of the top line

	// Constants from config file.  These are reasonable default values.
	cfg := map[string]float64{
		"W": 320, "H": 240,
		"ACH_DURATION": 15,    // How long a text blob may stay on screen before it starts scrolling off, in seconds
		"SCROLL_SPEED": 100.0, // in pixels per second
		"X_BORDER":     10, "Y_BORDER": 10,
	}
	colour := map[string]color.RGBA{
		"BACKGROUND": color.RGBA{0, 0, 0, 0xFF},
		"RECTANGLE":  color.RGBA{0, 0, 0xFF, 0xFF},
		"TEXT":       color.RGBA{0xFF, 0xFF, 0xFF, 0xFF},
	}
	ini_data, err := ini.Load("priv_ach.ini")
	if err == nil {
		sec := ini_data.Section("ui")
		for k := range cfg {
			if sec.HasKey(k) {
				f, err := sec.Key(k).Float64()
				if err == nil {
					cfg[k] = f
				}
			}
		}

		for k := range colour {
			if sec.HasKey(k + "_COLOUR") {
				col, err := color_from_string(sec.Key(k + "_COLOUR").String())
				if err != nil {
					fmt.Println(err)
				} else {
					colour[k] = col
				}
			}
		}
	}

	wcfg := opengl.WindowConfig{
		Title:  "Privateer achievements",
		Bounds: pixel.R(0, 0, cfg["W"], cfg["H"]),
		VSync:  true,
	}
	win, err := opengl.NewWindow(wcfg)
	if err != nil {
		panic(err)
	}
	defer win.Destroy()

	atlas := text.NewAtlas(basicfont.Face7x13, text.ASCII)
	texts := []*text.Text{}
	cheeves := make(chan *priv_ach.Achievement)
	expired := make(chan bool)
	watcher := priv_ach.New_watcher(utils.Get_savefile_dir())
	err = watcher.Start_watching(cheeves)
	if err != nil {
		fmt.Println(err)
	}
	defer watcher.Stop_watching() // somewhat unnecessary since there is not (yet) any quit function other than CTRL-C

	// Gap between two text blocks is a single LINE_HEIGHT
	// To have a proper border, background rectangle is LINE_HEIGHT/2 taller than this, centred so that there is LINE_HEIGHT/4 of rectangle above and below the text
	bgsprite := solid_rect_sprite(pixel.Rect{pixel.V(0, 0), pixel.V(cfg["W"]-cfg["X_BORDER"], TEXT_HEIGHT-LINE_HEIGHT/2)}, colour["RECTANGLE"])

	old_texts := 0        // number of text blobs that need to be scrolled up into oblivion (does not count partially scrolled)
	current_scroll := 0.0 // in pixels, positive number

	old_time := time.Now()
	for !win.Closed() {
		new_time := time.Now()
		tick := new_time.Sub(old_time)
		old_time = new_time

		for current_scroll >= 1 {
			// A text blob has scrolled off the top of the screen
			current_scroll -= 1
			texts = clean(texts[1:])
			old_texts -= 1
		}
		if old_texts >= 1 {
			// at least one text blob is too old, so scrolling should be happening
			current_scroll += tick.Seconds() * cfg["SCROLL_SPEED"] / float64(TEXT_HEIGHT)
		}

		select {

		case cheev := <-cheeves:
			ach_text := text.New(pixel.V(0, 0), atlas)
			ach_text.Color = colour["TEXT"]
			fmt.Fprintln(ach_text, cheev.Name)
			fmt.Fprintln(ach_text, cheev.Desc)
			fmt.Fprintln(ach_text, "Category:", cheev.Category)
			texts = append(texts, ach_text)

			time.AfterFunc(time.Duration(cfg["ACH_DURATION"]*float64(time.Second)), // UGH!!!!
				func() { expired <- true })

		case <-expired:
			old_texts += 1

		default:
			// DRAWING STARTS HERE

			win.Clear(colour["BACKGROUND"])
			y_base := cfg["H"] - cfg["Y_BORDER"] + current_scroll*float64(TEXT_HEIGHT)
			for i, t := range texts {
				y_base_i := y_base - float64(i*TEXT_HEIGHT)
				bgsprite.Draw(win, pixel.IM.Moved(pixel.V(cfg["W"]/2, y_base_i-TEXT_HEIGHT/2+LINE_HEIGHT/4)))
				t.Draw(win, pixel.IM.Moved(pixel.V(cfg["X_BORDER"], y_base_i-TEXT_H_OFFSET)))
			}
			win.Update()

			// DRAWING ENDS HERE
		}
	}
}
