package tui

import (
	"fmt"
	"os"
	"time"

	"golang.org/x/term"
)

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner prints an animated spinner to stderr while work is in progress.
// It is a no-op when stderr is not a terminal (e.g. piped output).
type Spinner struct {
	msg  string
	stop chan struct{}
	done chan struct{}
}

// Start launches the spinner goroutine and returns the Spinner.
func Start(msg string) *Spinner {
	s := &Spinner{
		msg:  msg,
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		close(s.done)
		return s
	}
	go s.run()
	return s
}

func (s *Spinner) run() {
	defer close(s.done)
	i := 0
	for {
		select {
		case <-s.stop:
			fmt.Fprint(os.Stderr, "\r\033[K") // clear the spinner line
			return
		default:
			fmt.Fprintf(os.Stderr, "\r\033[K  %s  %s",
				spinFrames[i%len(spinFrames)], s.msg)
			i++
			time.Sleep(80 * time.Millisecond)
		}
	}
}

// UpdateMsg changes the spinner label mid-flight.
func (s *Spinner) UpdateMsg(msg string) {
	s.msg = msg
}

// Stop clears the spinner and waits for the goroutine to exit.
func (s *Spinner) Stop() {
	select {
	case <-s.stop: // already stopped
	default:
		close(s.stop)
	}
	<-s.done
}
