package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

func arrowSelect(prompt string, options []string, defaultIdx int) (int, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return arrowSelectFallback(options, defaultIdx)
	}

	const visibleRows = 8
	fd := int(os.Stdin.Fd())

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return arrowSelectFallback(options, defaultIdx)
	}
	restore := func() {
		os.Stdout.WriteString("\033[?25h") // Mostra cursor
		term.Restore(fd, oldState)
	}
	defer restore()

	os.Stdout.WriteString("\033[?25l") // Esconde cursor

	cur := defaultIdx
	offset := 0

	clampOffset := func() {
		if cur < offset { offset = cur }
		if cur >= offset+visibleRows { offset = cur - visibleRows + 1 }
	}

	buildLines := func() []string {
		clampOffset()
		ls := make([]string, 0, 2+visibleRows+1)
		ls = append(ls,
			fmt.Sprintf("  \033[1m%s\033[0m", prompt),
			"  \033[2m↑↓ j/k · Enter to confirm\033[0m",
		)
		for i := offset; i < offset+visibleRows; i++ {
			if i < len(options) {
				if i == cur {
					colors := [3]int{48, 45, 63}
					ls = append(ls, fmt.Sprintf("  \033[1m\033[38;5;%dm▶ %s\033[0m", colors[cur%3], options[i]))
				} else {
					ls = append(ls, fmt.Sprintf("    %s", options[i]))
				}
			} else {
				ls = append(ls, "")
			}
		}
		end := offset + visibleRows
		if end > len(options) { end = len(options) }
		if len(options) > visibleRows {
			ls = append(ls, fmt.Sprintf("  \033[2m  %d–%d of %d\033[0m", offset+1, end, len(options)))
		} else {
			ls = append(ls, "")
		}
		return ls
	}

	draw := func(ls []string, first bool) {
		if !first {
			fmt.Fprintf(os.Stdout, "\r\033[%dA", len(ls)-1)
		}
		for i, l := range ls {
			os.Stdout.WriteString("\r\033[K" + l)
			if i < len(ls)-1 {
				os.Stdout.WriteString("\n")
			}
		}
	}

	lines := buildLines()
	draw(lines, true)

	buf := make([]byte, 6)
	for {
		n, _ := os.Stdin.Read(buf)
		b := buf[:n]

		moved := false
		switch {
		case len(b) == 1 && (b[0] == '\r' || b[0] == '\n'):
			// Limpa o menu ao confirmar e retorna o cursor ao início do bloco
			fmt.Fprintf(os.Stdout, "\r\033[%dA", len(lines)-1)
			for i := 0; i < len(lines); i++ {
				os.Stdout.WriteString("\r\033[K")
				if i < len(lines)-1 { os.Stdout.WriteString("\n") }
			}
			fmt.Fprintf(os.Stdout, "\r\033[%dA", len(lines)-1)
			restore()
			label := stripANSI(options[cur])
			fmt.Printf("\033[1m%s\033[0m\n", label)
			return cur, nil

		case len(b) == 1 && b[0] == 3: // Ctrl-C
			restore()
			fmt.Println()
			return 0, fmt.Errorf("cancelled")

		case len(b) == 1 && (b[0] == 'k' || b[0] == 'K') || (len(b) >= 3 && b[0] == 27 && b[1] == '[' && b[2] == 'A'):
			if cur > 0 { cur--; moved = true }
		case len(b) == 1 && (b[0] == 'j' || b[0] == 'J') || (len(b) >= 3 && b[0] == 27 && b[1] == '[' && b[2] == 'B'):
			if cur < len(options)-1 { cur++; moved = true }
		}

		if moved {
			lines = buildLines()
			draw(lines, false)
		}
	}
}

// arrowSelectWithDetails is like arrowSelect but shows a detail line below
// the selected item (e.g. benchmark scores). details[i] may be empty.
func arrowSelectWithDetails(prompt string, options, details []string, defaultIdx int) (int, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return arrowSelectFallback(options, defaultIdx)
	}

	const visibleRows = 8
	fd := int(os.Stdin.Fd())

	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return arrowSelectFallback(options, defaultIdx)
	}
	restore := func() {
		os.Stdout.WriteString("\033[?25h")
		term.Restore(fd, oldState)
	}
	defer restore()

	os.Stdout.WriteString("\033[?25l")

	cur := defaultIdx
	offset := 0

	clampOffset := func() {
		if cur < offset {
			offset = cur
		}
		if cur >= offset+visibleRows {
			offset = cur - visibleRows + 1
		}
	}

	buildLines := func() []string {
		clampOffset()
		ls := make([]string, 0, 3+visibleRows+2)
		ls = append(ls,
			fmt.Sprintf("  \033[1m%s\033[0m", prompt),
			"  \033[2m↑↓ j/k · Enter to confirm\033[0m",
		)
		for i := offset; i < offset+visibleRows; i++ {
			if i < len(options) {
				if i == cur {
					colors := [3]int{48, 45, 63}
					ls = append(ls, fmt.Sprintf("  \033[1m\033[38;5;%dm▶ %s\033[0m", colors[cur%3], options[i]))
				} else {
					ls = append(ls, fmt.Sprintf("    %s", options[i]))
				}
			} else {
				ls = append(ls, "")
			}
		}
		end := offset + visibleRows
		if end > len(options) {
			end = len(options)
		}
		if len(options) > visibleRows {
			ls = append(ls, fmt.Sprintf("  \033[2m  %d–%d of %d\033[0m", offset+1, end, len(options)))
		} else {
			ls = append(ls, "")
		}
		// Detail line for selected item.
		detail := ""
		if cur < len(details) {
			detail = details[cur]
		}
		if detail != "" {
			ls = append(ls, fmt.Sprintf("  \033[2m  %s\033[0m", detail))
		} else {
			ls = append(ls, "")
		}
		return ls
	}

	draw := func(ls []string, first bool) {
		if !first {
			fmt.Fprintf(os.Stdout, "\r\033[%dA", len(ls)-1)
		}
		for i, l := range ls {
			os.Stdout.WriteString("\r\033[K" + l)
			if i < len(ls)-1 {
				os.Stdout.WriteString("\n")
			}
		}
	}

	lines := buildLines()
	draw(lines, true)

	buf := make([]byte, 6)
	for {
		n, _ := os.Stdin.Read(buf)
		b := buf[:n]

		moved := false
		switch {
		case len(b) == 1 && (b[0] == '\r' || b[0] == '\n'):
			fmt.Fprintf(os.Stdout, "\r\033[%dA", len(lines)-1)
			for i := 0; i < len(lines); i++ {
				os.Stdout.WriteString("\r\033[K")
				if i < len(lines)-1 {
					os.Stdout.WriteString("\n")
				}
			}
			fmt.Fprintf(os.Stdout, "\r\033[%dA", len(lines)-1)
			restore()
			label := stripANSI(options[cur])
			fmt.Printf("\033[1m%s\033[0m\n", label)
			return cur, nil

		case len(b) == 1 && b[0] == 3:
			restore()
			fmt.Println()
			return 0, fmt.Errorf("cancelled")

		case len(b) == 1 && (b[0] == 'k' || b[0] == 'K') || (len(b) >= 3 && b[0] == 27 && b[1] == '[' && b[2] == 'A'):
			if cur > 0 {
				cur--
				moved = true
			}
		case len(b) == 1 && (b[0] == 'j' || b[0] == 'J') || (len(b) >= 3 && b[0] == 27 && b[1] == '[' && b[2] == 'B'):
			if cur < len(options)-1 {
				cur++
				moved = true
			}
		}

		if moved {
			lines = buildLines()
			draw(lines, false)
		}
	}
}

func arrowSelectFallback(options []string, defaultIdx int) (int, error) {
	for i, o := range options {
		marker := "  "
		if i == defaultIdx { marker = "▶ " }
		fmt.Printf("  %s[%d] %s\n", marker, i+1, o)
	}
	fmt.Printf("\n  Choice [%d]: ", defaultIdx+1)
	var val string
	fmt.Scanln(&val)
	val = strings.TrimSpace(val)
	if val == "" { return defaultIdx, nil }
	for i := range options {
		if val == fmt.Sprintf("%d", i+1) { return i, nil }
	}
	return defaultIdx, nil
}
