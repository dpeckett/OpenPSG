package termutil

import "fmt"

// ClearLines clears n lines from the terminal.
func ClearLines(n int) {
	// Move the cursor up n rows
	fmt.Printf("\033[%dA", n)
	// Clear each of these rows
	for i := 0; i < n; i++ {
		fmt.Print("\033[2K\r")
		// Move the cursor down to the next line after the first
		if i < n-1 {
			fmt.Print("\033[1B")
		}
	}
	// Position the cursor back at the beginning of the first cleared line
	fmt.Printf("\033[%dA", n-1)
}
