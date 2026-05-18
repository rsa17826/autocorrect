package main

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	argparse "github.com/rsa17826/go-arg-lib"
	"github.com/rsa17826/go-input-lib"
	"github.com/segmentio/encoding/json"
)

type WireEvent struct {
	Sec   int64
	Usec  int64
	Type  uint16
	Code  uint16
	Value int32
}

//go:embed corrections.json
var defaultConfigFileBytes []byte

var NORMAL = map[int]byte{
	input.KEY_A:          'a',
	input.KEY_B:          'b',
	input.KEY_C:          'c',
	input.KEY_D:          'd',
	input.KEY_E:          'e',
	input.KEY_F:          'f',
	input.KEY_G:          'g',
	input.KEY_H:          'h',
	input.KEY_I:          'i',
	input.KEY_J:          'j',
	input.KEY_K:          'k',
	input.KEY_L:          'l',
	input.KEY_M:          'm',
	input.KEY_N:          'n',
	input.KEY_O:          'o',
	input.KEY_P:          'p',
	input.KEY_Q:          'q',
	input.KEY_R:          'r',
	input.KEY_S:          's',
	input.KEY_T:          't',
	input.KEY_U:          'u',
	input.KEY_V:          'v',
	input.KEY_W:          'w',
	input.KEY_X:          'x',
	input.KEY_Y:          'y',
	input.KEY_Z:          'z',
	input.KEY_0:          '0',
	input.KEY_1:          '1',
	input.KEY_2:          '2',
	input.KEY_3:          '3',
	input.KEY_4:          '4',
	input.KEY_5:          '5',
	input.KEY_6:          '6',
	input.KEY_7:          '7',
	input.KEY_8:          '8',
	input.KEY_9:          '9',
	input.KEY_SPACE:      ' ',
	input.KEY_MINUS:      '-',
	input.KEY_EQUAL:      '=',
	input.KEY_LEFTBRACE:  '[',
	input.KEY_RIGHTBRACE: ']',
	input.KEY_SEMICOLON:  ';',
	input.KEY_APOSTROPHE: '\'',
	input.KEY_GRAVE:      '`',
	input.KEY_BACKSLASH:  '\\',
	input.KEY_COMMA:      ',',
	input.KEY_DOT:        '.',
	input.KEY_SLASH:      '/',
	input.KEY_ENTER:      '\n',
	input.KEY_KPENTER:    '\n',
}

var SHIFTED = map[int]byte{
	input.KEY_A:          'A',
	input.KEY_B:          'B',
	input.KEY_C:          'C',
	input.KEY_D:          'D',
	input.KEY_E:          'E',
	input.KEY_F:          'F',
	input.KEY_G:          'G',
	input.KEY_H:          'H',
	input.KEY_I:          'I',
	input.KEY_J:          'J',
	input.KEY_K:          'K',
	input.KEY_L:          'L',
	input.KEY_M:          'M',
	input.KEY_N:          'N',
	input.KEY_O:          'O',
	input.KEY_P:          'P',
	input.KEY_Q:          'Q',
	input.KEY_R:          'R',
	input.KEY_S:          'S',
	input.KEY_T:          'T',
	input.KEY_U:          'U',
	input.KEY_V:          'V',
	input.KEY_W:          'W',
	input.KEY_X:          'X',
	input.KEY_Y:          'Y',
	input.KEY_Z:          'Z',
	input.KEY_0:          ')',
	input.KEY_1:          '!',
	input.KEY_2:          '@',
	input.KEY_3:          '#',
	input.KEY_4:          '$',
	input.KEY_5:          '%',
	input.KEY_6:          '^',
	input.KEY_7:          '&',
	input.KEY_8:          '*',
	input.KEY_9:          '(',
	input.KEY_SPACE:      ' ',
	input.KEY_MINUS:      '_',
	input.KEY_EQUAL:      '+',
	input.KEY_LEFTBRACE:  '{',
	input.KEY_RIGHTBRACE: '}',
	input.KEY_SEMICOLON:  ':',
	input.KEY_APOSTROPHE: '"',
	input.KEY_GRAVE:      '~',
	input.KEY_BACKSLASH:  '|',
	input.KEY_COMMA:      '<',
	input.KEY_DOT:        '>',
	input.KEY_SLASH:      '?',
	input.KEY_ENTER:      '\n',
	input.KEY_KPENTER:    '\n',
}
var RESET_KEYS = []int{
	input.KEY_LEFT,
	input.KEY_RIGHT,
	input.KEY_UP,
	input.KEY_DOWN,
	input.KEY_HOME,
	input.KEY_END,
	input.KEY_PAGEUP,
	input.KEY_PAGEDOWN,
	input.KEY_DELETE,
	input.KEY_ESC,
}

type CorrectionsConfig map[string]string

var shiftHeld bool
var ctrlHeld bool
var altHeld bool
var metaHeld bool
var capslockOn bool

func main() {
	var capsHasBeenDisabled bool
	var correctionsPath string
	argparse.ParseArgs([]argparse.ArgumentData{
		{Keys: []string{"capsHasBeenDisabled"}, AfterCount: 0, Target: &capsHasBeenDisabled, Description: "caps is not used to toggle the case state so don't detect use of the capslock button as if it does that"},
		{Keys: []string{"corrections"}, AfterCount: 1, Target: &correctionsPath, Description: "Path to corrections JSON file", Default: []any{filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "corrections.json")}},
	})
	if _, err := os.Stat(correctionsPath); os.IsNotExist(err) {
		_ = os.WriteFile(correctionsPath, defaultConfigFileBytes, 0644)
	}

	// 2. Open the file first (this returns an *os.File, which implements io.Reader)
	byteValue, err := os.ReadFile(correctionsPath)
	if err != nil {
		log.Fatalf("Failed to read file: %v", err)
	}

	// 4. Parse (Unmarshal) the JSON into a Go struct or map
	var corrections CorrectionsConfig
	err = json.Unmarshal(byteValue, &corrections)
	if err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	conn, err := net.Dial("unix", "/tmp/kbd_manager.sock")
	if err != nil {
		panic(err)
	}
	defer conn.Close()
	fmt.Fprint(conn, "FILTER\n")
	var ev WireEvent
	evSize := binary.Size(ev)
	buf := make([]byte, evSize)
	buffer := make([]byte, 0, 150)

	for {
		_, err := io.ReadFull(conn, buf)
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				fmt.Println("Manager closed the connection.")
			} else {
				fmt.Fprintf(os.Stderr, "Error reading wire event: %v\n", err)
			}
			break
		}
		if err := binary.Read(bytes.NewReader(buf), binary.LittleEndian, &ev); err != nil {
			fmt.Fprintf(os.Stderr, "Error decoding wire event: %v\n", err)
			break
		}

		const TRIGGER_CHARS = " \t\n-()[]{}';:/\\,.?!@#$%^&*+=<>|`~\""
		const BUFFER_MAX int = 150
		var modify = func() bool {
			if ev.Type == input.EV_KEY {
				println(buffer, len(corrections))
				switch ev.Code {
				case input.KEY_LEFTSHIFT, input.KEY_RIGHTSHIFT:
					{
						shiftHeld = ev.Type != 0
						return false
					}
				case input.KEY_LEFTCTRL, input.KEY_RIGHTCTRL:
					{
						ctrlHeld = ev.Type != 0
						return false
					}
				case input.KEY_LEFTALT, input.KEY_RIGHTALT:
					{
						altHeld = ev.Type != 0
						return false
					}
				case input.KEY_LEFTMETA, input.KEY_RIGHTMETA:
					{
						metaHeld = ev.Type != 0
						return false
					}
				case input.KEY_CAPSLOCK:
					{
						if !capsHasBeenDisabled {
							capslockOn = ev.Type != 0
							return false
						}
					}
				case input.KEY_BACKSPACE:
					{
						if len(buffer) > 0 {
							buffer = buffer[:len(buffer)-1]
						}
						return false
					}
				default:
					{
						if ev.Code > 247 {
							return false
						}
						if ctrlHeld || altHeld || metaHeld {
							buffer = buffer[:0]
							return false
						}

						var table map[int]byte
						if shiftHeld != capslockOn {
							table = SHIFTED
						} else {
							table = NORMAL
						}

						char, exists := table[int(ev.Code)]
						if !exists || char == 0 {
							// If the key pressed isn't in our alphabet maps, skip it
							return false
						}

						// strings.Contains needs a string, so we convert our single byte 'char' to string
						if strings.Contains(TRIGGER_CHARS, string(char)) {
							for wrong, right := range corrections {
								// Convert the 'wrong' string to []byte to use bytes.HasSuffix
								if bytes.HasSuffix(buffer, []byte(wrong)) {
									isStartOfWord := false
									wrongLen := len(wrong)
									bufLen := len(buffer)

									if bufLen == wrongLen {
										isStartOfWord = true
									} else {
										// Calculate correct positive indices from the back of the buffer
										prev := buffer[bufLen-wrongLen-1]
										curr := buffer[bufLen-wrongLen]

										// Safety check: make sure next doesn't go out of bounds
										var next byte
										if bufLen-wrongLen+1 < bufLen {
											next = buffer[bufLen-wrongLen+1]
										}

										// 1. Check if previous character is not a letter
										if !unicode.IsLetter(rune(prev)) {
											isStartOfWord = true
										}
										// 2. Camel/Pascal start (lower followed by Upper: e.g., a|B)
										if unicode.IsLower(rune(prev)) || unicode.IsUpper(rune(curr)) {
											isStartOfWord = true
										}
										// 3. Acronym boundary (Upper followed by Upper then lower: e.g., L|Pa)
										if next != 0 && unicode.IsUpper(rune(prev)) && unicode.IsUpper(rune(curr)) && unicode.IsLower(rune(next)) {
											isStartOfWord = true
										}
									}

									if isStartOfWord {
										apply_correction(wrong, right, rune(char))

										// Safely reconstruct the buffer using append()
										// Slice out the 'wrong' word, then append 'right' (cast to bytes), then append the new 'char'
										buffer = buffer[:bufLen-wrongLen]
										buffer = append(buffer, []byte(right)...)
										buffer = append(buffer, char)

										if len(buffer) > BUFFER_MAX {
											buffer = buffer[len(buffer)-BUFFER_MAX:]
										}
										return true
									}
								}
							}
						}

						buffer = append(buffer, char)
						if len(buffer) > BUFFER_MAX {
							buffer = buffer[len(buffer)-BUFFER_MAX:]
						}
					}
				}
			}
			return false
		}()
		// Response byte loop back to manager
		if modify {
			_, err = conn.Write([]byte{1})
		} else {
			_, err = conn.Write([]byte{0})
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send filter response byte: %v\n", err)
			break
		}
	}
}

func apply_correction(wrong, right string, triggerChar rune) {
	events := make([]WireEvent, 0)
	var lastUsedShift bool = shiftHeld
	for range wrong {
		events = append(events, []WireEvent{
			{
				Type:  input.EV_KEY,
				Code:  input.KEY_BACKSPACE,
				Value: int32(1),
			},
			{
				Type:  input.EV_KEY,
				Code:  input.KEY_BACKSPACE,
				Value: int32(0),
			},
		}...)
	}
	for _, char := range right {
		keyInfo := input.CharKeyMap[char]
		if keyInfo.Shift != lastUsedShift {
			if keyInfo.Shift {
				events = append(events, []WireEvent{
					{
						Type:  input.EV_KEY,
						Code:  keyInfo.Code,
						Value: int32(1),
					},
				}...)
			} else {
				events = append(events, []WireEvent{
					{
						Type:  input.EV_KEY,
						Code:  keyInfo.Code,
						Value: int32(0),
					},
				}...)
			}
		}
		events = append(events, []WireEvent{
			{
				Type:  input.EV_KEY,
				Code:  keyInfo.Code,
				Value: int32(1),
			},
			{
				Type:  input.EV_KEY,
				Code:  keyInfo.Code,
				Value: int32(0),
			},
		}...)
	}
	if lastUsedShift != shiftHeld {
		if shiftHeld {
			events = append(events, []WireEvent{
				{
					Type:  input.EV_KEY,
					Code:  input.KEY_LEFTSHIFT,
					Value: int32(1),
				},
			}...)
		} else {
			events = append(events, []WireEvent{
				{
					Type:  input.EV_KEY,
					Code:  input.KEY_LEFTSHIFT,
					Value: int32(0),
				},
			}...)
		}
		events = append(events, []WireEvent{
			{
				Type:  input.EV_KEY,
				Code:  input.CharKeyMap[triggerChar].Code,
				Value: int32(1),
			},
			{
				Type:  input.EV_KEY,
				Code:  input.CharKeyMap[triggerChar].Code,
				Value: int32(0),
			},
		}...)
	}
	conn, err := net.Dial("unix", "/tmp/kbd_manager.sock")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Initialize context registration handshake
	fmt.Fprint(conn, "INJECT\n")
	for _, stroke := range events {
		binary.Write(conn, binary.LittleEndian, stroke)
	}
}

// return correct shift side to pressed if required

// func send() {
// 	conn, err := net.Dial("unix", "/tmp/kbd_manager.sock")
// 	if err != nil {
// 		panic(err)
// 	}
// 	defer conn.Close()

// 	// Initialize context registration handshake
// 	fmt.Fprint(conn, "INJECT\n")

// 	// // Frame an injection event packet (e.g., Force Mouse right relative +100 units)
// 	// if x != 0 {
// 	// 	stroke := WireEvent{
// 	// 		Type:  input.EV_REL, // EV_REL
// 	// 		Code:  input.REL_X,  // REL_X
// 	// 		Value: int32(x),     // Move right 100 pixels
// 	// 	}

// 	// 	binary.Write(conn, binary.LittleEndian, stroke)
// 	// }
// 	// if y != 0 {
// 	// 	stroke := WireEvent{
// 	// 		Type:  input.EV_REL, // EV_REL
// 	// 		Code:  input.REL_Y,  // REL_X
// 	// 		Value: int32(y),     // Move right 100 pixels
// 	// 	}

// 	// 	binary.Write(conn, binary.LittleEndian, stroke)
// 	// }
// }
