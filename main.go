package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unicode"

	argparse "github.com/rsa17826/go-arg-lib"
	"github.com/rsa17826/go-input-lib"
	"github.com/rsa17826/input-manager/IMan"
	"github.com/segmentio/encoding/json"
)

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

// CorrectionEntry is the parsed, typed form of a single corrections.json entry.
type CorrectionEntry struct {
	ReplaceWith                string
	ForceCaseMatch             bool
	NoEndActionRequired        bool
	AllowTriggeringInsideWords bool
	Action                     string // "replace" | "clear buffer" | "error" | ""
}

func parseCorrectionsConfig(raw map[string]any) (endActionRequired, anywhere map[string]CorrectionEntry) {
	endActionRequired = make(map[string]CorrectionEntry, len(raw))
	anywhere = make(map[string]CorrectionEntry, 0)

	for wrong, right := range raw {
		var entry CorrectionEntry
		entry.ForceCaseMatch = true // default
		switch v := right.(type) {
		case string:
			entry.ReplaceWith = v
			entry.Action = "replace"
			entry.ForceCaseMatch = true
			entry.NoEndActionRequired = false
			entry.AllowTriggeringInsideWords = false

		case map[string]any:
			if actionVal, ok := v["action"].(string); ok {
				switch actionVal {
				case "replace", "clear buffer":
					entry.Action = actionVal
				default:
					entry.Action = "error"
				}
			} else {
				entry.Action = "replace"
			}
			if s, ok := v["replace"].(string); ok {
				entry.ReplaceWith = s
			} else if entry.Action == "replace" {
				entry.Action = "error"
			}
			if fcm, ok := v["forceCaseMatch"].(bool); ok {
				entry.ForceCaseMatch = fcm
			}
			if near, ok := v["noEndActionRequired"].(bool); ok {
				entry.NoEndActionRequired = near
			}
			if atiw, ok := v["allowTriggeringInsideWords"].(bool); ok {
				entry.AllowTriggeringInsideWords = atiw
			}
		default:
			entry.Action = "error"
		}

		if entry.NoEndActionRequired {
			anywhere[wrong] = entry
		} else {
			endActionRequired[wrong] = entry
		}
	}
	return endActionRequired, anywhere
}

var correcting atomic.Int32

var capslockOn bool
var conn IMan.ManagerConnection

func main() {
	var capsHasBeenDisabled bool
	var correctionsPath string
	argparse.ParseArgs([]argparse.ArgumentData{
		{Keys: []string{"capsHasBeenDisabled"}, AfterCount: 0, Target: &capsHasBeenDisabled, Description: "caps is not used to toggle the case state so don't detect use of the capslock button as if it does that"},
		{Keys: []string{"corrections"}, AfterCount: 1, Target: &correctionsPath, Description: "Path to corrections JSON file", Default: []any{filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "autocorrect_daemon", "corrections.json")}},
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
	var rawCorrections map[string]any
	err = json.Unmarshal(byteValue, &rawCorrections)
	if err != nil {
		log.Fatalf("Failed to parse JSON: %v", err)
	}

	endActionRequiredConnections, anywhereCorrections := parseCorrectionsConfig(rawCorrections)

	conn, err := IMan.Connect("autocorrect", IMan.ModeFilter)
	if err != nil {
		panic(err)
	}
	err = conn.EnableKeyMap(false)
	if err != nil {
		panic(err)
	}
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM, syscall.SIGABRT)
		<-sigChan
		conn.Close()
		os.Exit(1)
	}()

	var ev IMan.WireEvent
	buffer := make([]byte, 0, 150)

	for {
		resp, err := conn.ReadNext()
		if err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				time.Sleep(1 * time.Second)
				fmt.Println("Manager closed the connection.")
			} else {
				fmt.Fprintf(os.Stderr, "Error reading wire event: %v\n", err)
			}
			os.Exit(1)
		}
		ev = resp.Event

		const TRIGGER_CHARS = " \t\n-()[]{}';:/\\,.?!@#$%^&*+=<>|`~\""
		const BUFFER_MAX int = 150

		var foundMatchingEntry bool
		// FIX 1: Explicitly verify this is a keyboard driver action event
		if ev.Type == input.EV_KEY {
			// Value == 1 is Key Press, Value == 2 is Key Repeat. Value == 0 is Key Release!
			// We handle Modifiers on both Press AND Release, but ignore text updates on Release.
			isKeyPress := (ev.Value == 1 || ev.Value == 2)

			switch ev.Code {
			case input.KEY_CAPSLOCK:
				if !capsHasBeenDisabled && isKeyPress {
					capslockOn = !capslockOn
				}
			default:
				if isKeyPress && ev.Code <= 247 {
					if conn.AltPressedReal() || conn.CtrlPressedReal() || conn.MetaPressedReal() {
						buffer = buffer[:0]
					} else {
						if ev.Code == input.KEY_BACKSPACE {
							if len(buffer) > 0 {
								buffer = buffer[:len(buffer)-1]
							}
						}
						var table map[int]byte
						if conn.ShiftPressedReal() != capslockOn {
							table = SHIFTED
						} else {
							table = NORMAL
						}

						char, exists := table[int(ev.Code)]
						if exists && char != 0 {
							tryCorrect := func(table map[string]CorrectionEntry) (bool, []byte) {
								for wrong, entry := range table {
									var testBuffer []byte
									if entry.NoEndActionRequired {
										testBuffer = append(buffer, char)
									} else {
										testBuffer = buffer
									}
									if entry.ForceCaseMatch {
										if !bytes.HasSuffix(testBuffer, []byte(wrong)) {
											continue
										}
									} else {
										if !bytes.HasSuffix(bytes.ToLower(testBuffer), bytes.ToLower([]byte(wrong))) {
											continue
										}
									}
									wrongLen := len(wrong)
									bufLen := len(testBuffer)
									println("?Correcting:", wrong, "->", entry.ReplaceWith,
										"forceCaseMatch", entry.ForceCaseMatch,
										"noEndActionRequired", entry.NoEndActionRequired,
										"allowTriggeringInsideWords", entry.AllowTriggeringInsideWords,
										"action", entry.Action)
									// println("bufLen", bufLen, wrongLen)
									if !entry.AllowTriggeringInsideWords {
										isStartOfWord := false
										if bufLen == wrongLen {
											isStartOfWord = true
										} else {
											prev := testBuffer[bufLen-wrongLen-1]
											curr := testBuffer[bufLen-wrongLen]
											var next byte
											if bufLen-wrongLen+1 < bufLen {
												next = testBuffer[bufLen-wrongLen+1]
											}
											if !unicode.IsLetter(rune(prev)) {
												isStartOfWord = true
											}
											if unicode.IsLower(rune(prev)) && unicode.IsUpper(rune(curr)) {
												isStartOfWord = true
											}
											if next != 0 && unicode.IsUpper(rune(prev)) && unicode.IsUpper(rune(curr)) && unicode.IsLower(rune(next)) {
												isStartOfWord = true
											}
										}
										if !isStartOfWord {
											continue
										}
									}

									println("!Correcting:", wrong, "->", entry.ReplaceWith,
										"forceCaseMatch", entry.ForceCaseMatch,
										"noEndActionRequired", entry.NoEndActionRequired,
										"allowTriggeringInsideWords", entry.AllowTriggeringInsideWords,
										"action", entry.Action)

									switch entry.Action {
									case "replace":
										{
											correcting.Store(1)
											_, err = conn.BlockInput(1)
											if err != nil {
												fmt.Fprintf(os.Stderr, "Failed to send filter response byte: %v\n", err)
												return true, buffer
											}
											rightWord := entry.ReplaceWith
											go apply_correction(wrong, rightWord, rune(char), entry)
											buffer = buffer[:bufLen-wrongLen]
											buffer = append(buffer, []byte(entry.ReplaceWith)...)
											buffer = append(buffer, char)
											if len(buffer) > BUFFER_MAX {
												buffer = buffer[len(buffer)-BUFFER_MAX:]
											}
											return true, buffer
										}
									case "clear buffer":
										{
											_, err = conn.BlockInput(0)
											if err != nil {
												fmt.Fprintf(os.Stderr, "Failed to send filter response byte: %v\n", err)
												return true, buffer
											}
											buffer = buffer[:0]
											return true, buffer
										}
									default:
										{
											println("ERROR: entry.Action: ", entry.Action, wrong, entry.ReplaceWith)
											return true, buffer
										}
									}
								}
								return false, buffer
							}
							if strings.Contains(TRIGGER_CHARS, string(char)) {
								foundMatchingEntry, buffer = tryCorrect(anywhereCorrections)
								if !foundMatchingEntry {
									foundMatchingEntry, buffer = tryCorrect(endActionRequiredConnections)
								}
							} else {
								foundMatchingEntry, buffer = tryCorrect(anywhereCorrections)
							}
							println("foundMatchingEntry", foundMatchingEntry)
							if !foundMatchingEntry {
								buffer = append(buffer, char)
								if len(buffer) > BUFFER_MAX {
									buffer = buffer[len(buffer)-BUFFER_MAX:]
								}
							}
						}
					}
					// println(fmt.Sprintf("[%s]", buffer), len(endActionRequiredConnections), len(anywhereCorrections))
				}
			}
		}

		// Response byte loop back out to manager
		if !foundMatchingEntry {
			resp := byte(0)
			if ev.Value != 0 && correcting.Load() == 1 {
				resp = 1 // block ALL events while correction is in flight
			}
			_, err = conn.BlockInput(resp)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to send filter response byte: %v\n", err)
			// os.Exit(1)
		}
	}
}

func apply_correction(wrong, right string, triggerChar rune, entry CorrectionEntry) {
	// println("asdjkasdjkasdjkads", wrong, right, triggerChar, entry.NoEndActionRequired, conn.PressedKeys())
	correcting.Store(1)
	defer correcting.Store(0)
	events := make([]IMan.WireEvent, 0)
	var lastUsedShift bool = conn.ShiftPressedReal()
	backspaces := len(wrong)

	if entry.NoEndActionRequired && entry.Action == "replace" {
		backspaces--
	}

	for range backspaces {
		events = append(events, []IMan.WireEvent{
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
	// events = append(events, []IMan.WireEvent{
	// 	{},
	// }...)
	for _, char := range right {
		keyInfo := input.CharKeyMap[char]
		if keyInfo.Shift != lastUsedShift {
			if keyInfo.Shift {
				events = append(events, []IMan.WireEvent{
					{
						Type:  input.EV_KEY,
						Code:  input.KEY_LEFTSHIFT,
						Value: int32(1),
					},
					// {},
				}...)
			} else {
				events = append(events, []IMan.WireEvent{
					{
						Type:  input.EV_KEY,
						Code:  input.KEY_LEFTSHIFT,
						Value: int32(0),
					},
					// {},
				}...)
			}
			lastUsedShift = keyInfo.Shift
		}
		events = append(events, []IMan.WireEvent{
			{
				Type:  input.EV_KEY,
				Code:  keyInfo.Code,
				Value: int32(0),
			},
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
	if lastUsedShift != conn.ShiftPressedReal() {
		if conn.ShiftPressedReal() {
			events = append(events, []IMan.WireEvent{
				{
					Type:  input.EV_KEY,
					Code:  input.KEY_LEFTSHIFT,
					Value: int32(1),
				},
			}...)
		} else {
			events = append(events, []IMan.WireEvent{
				{
					Type:  input.EV_KEY,
					Code:  input.KEY_LEFTSHIFT,
					Value: int32(0),
				},
			}...)
		}
	}
	if !entry.NoEndActionRequired {
		events = append(events, []IMan.WireEvent{
			// {},
			{
				Type:  input.EV_KEY,
				Code:  input.CharKeyMap[triggerChar].Code,
				Value: int32(0),
			},
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
			// {},
		}...)
	}
	conn, err := net.Dial("unix", "/tmp/kbd_manager.sock")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	// Initialize context registration handshake
	im, err := IMan.Connect("autocorrect", IMan.ModeInjection)
	if err != nil {
		panic(err)
	}
	for i, stroke := range events {
		im.Send(stroke)
		// binary.Write(conn, binary.LittleEndian, stroke)
		// binary.Write(conn, binary.LittleEndian, IMan.WireEvent{})
		if i != 0 && i%10 == 0 {
			time.Sleep(1 * time.Millisecond)
		}
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
// 	// 	stroke := IMan.WireEvent{
// 	// 		Type:  input.EV_REL, // EV_REL
// 	// 		Code:  input.REL_X,  // REL_X
// 	// 		Value: int32(x),     // Move right 100 pixels
// 	// 	}

// 	// 	binary.Write(conn, binary.LittleEndian, stroke)
// 	// }
// 	// if y != 0 {
// 	// 	stroke := IMan.WireEvent{
// 	// 		Type:  input.EV_REL, // EV_REL
// 	// 		Code:  input.REL_Y,  // REL_X
// 	// 		Value: int32(y),     // Move right 100 pixels
// 	// 	}

// 	// 	binary.Write(conn, binary.LittleEndian, stroke)
// 	// }
// }
