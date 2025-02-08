/* SPDX-License-Identifier: AGPL-3.0-or-later
 *
 * Copyright (C) 2025 The OpenPSG Authors.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU Affero General Public License as published
 * by the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Affero General Public License for more details.
 *
 * You should have received a copy of the GNU Affero General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

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
