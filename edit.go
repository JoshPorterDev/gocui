// Copyright 2014 The gocui Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gocui

import (
	"errors"
)

// Editor interface must be satisfied by gocui editors.
type Editor interface {
	Edit(v *View, key Key, ch rune, mod Modifier)
}

// The EditorFunc type is an adapter to allow the use of ordinary functions as
// Editors. If f is a function with the appropriate signature, EditorFunc(f)
// is an Editor object that calls f.
type EditorFunc func(v *View, key Key, ch rune, mod Modifier)

// Edit calls f(v, key, ch, mod)
func (f EditorFunc) Edit(v *View, key Key, ch rune, mod Modifier) {
	f(v, key, ch, mod)
}

// DefaultEditor is the default editor.
var DefaultEditor Editor = EditorFunc(simpleEditor)

// simpleEditor is used as the default gocui editor.
func simpleEditor(v *View, key Key, ch rune, mod Modifier) {
	if ch != 0 && mod == 0 {
		v.EditWrite(ch)
		return
	}

	switch key {
	case KeySpace:
		v.EditWrite(' ')
	case KeyBackspace, KeyBackspace2:
		v.EditDelete(true)
	case KeyDelete:
		v.EditDelete(false)
	case KeyInsert:
		v.Overwrite = !v.Overwrite
	case KeyEnter:
		v.EditNewLine()
	case KeyArrowDown:
		v.MoveCursor(0, 1)
	case KeyArrowUp:
		v.MoveCursor(0, -1)
	case KeyArrowLeft:
		v.MoveCursor(-1, 0)
	case KeyArrowRight:
		v.MoveCursor(1, 0)
	case KeyTab:
		v.EditWrite('\t')
	case KeyEsc:
		// If not here the esc key will act like the KeySpace
	default:
		v.EditWrite(ch)
	}
}

// EditWrite writes a rune at the cursor position.
func (v *View) EditWrite(ch rune) {
	v.writeRune(v.cx, v.cy, ch)
	v.MoveCursor(1, 0)
}

// EditDeleteToStartOfLine is the equivalent of pressing ctrl+U in your terminal, it deletes to the start of the line. Or if you are already at the start of the line, it deletes the newline character
func (v *View) EditDeleteToStartOfLine() {
	x, _ := v.Cursor()
	if x == 0 {
		v.EditDelete(true)
	} else {
		// delete characters until we are the start of the line
		for x > 0 {
			v.EditDelete(true)
			x, _ = v.Cursor()
		}
	}
}

// EditGotoToStartOfLine takes you to the start of the current line
func (v *View) EditGotoToStartOfLine() {
	x, _ := v.Cursor()
	for x > 0 {
		v.MoveCursor(-1, 0)
		x, _ = v.Cursor()
	}
}

// EditGotoToEndOfLine takes you to the end of the line
func (v *View) EditGotoToEndOfLine() {
	_, y := v.Cursor()
	_ = v.SetCursor(0, y+1)
	x, newY := v.Cursor()
	if newY == y {
		// we must be on the last line, so lets move to the very end
		prevX := -1
		for prevX != x {
			prevX = x
			v.MoveCursor(1, 0)
			x, _ = v.Cursor()
		}
	} else {
		// most left so now we're at the end of the original line
		v.MoveCursor(-1, 0)
	}
}

// EditDelete deletes a rune at the cursor position. back determines the
// direction.
func (v *View) EditDelete(back bool) {
	x, y := v.cx, v.cy
	if y < 0 {
		return
	}
	if y >= len(v.lines) {
		v.MoveCursor(-1, 0)
		return
	}

	if back && x <= 0 { // start of the line
		if y <= 0 {
			// No reasone to merge lines
			return
		}

		previousLine := v.cy - 1
		v.MoveCursor(-1, 0)
		_ = v.mergeLines(previousLine)
		return
	}
	if back { // middle/end of the line
		if err := v.deleteRune(v.cx-1, v.cy); err == nil {
			v.MoveCursor(-1, 0)
		}
		return
	}
	if x == len(v.lines[y]) { // end of the line
		_ = v.mergeLines(y)
		return
	}
	v.deleteRune(v.cx, v.cy) // start/middle of the line
}

// EditNewLine inserts a new line under the cursor.
func (v *View) EditNewLine() {
	v.breakLine(v.cx, v.cy)
	v.ox = 0
	v.cy = v.cy + 1
	v.cx = 0
}

// MoveCursor moves the cursor relative from it's current possition
func (v *View) MoveCursor(dx, dy int) {
	newX, newY := v.cx+dx, v.cy+dy

	if len(v.lines) == 0 {
		v.cx, v.cy = 0, 0
		return
	}

	// If newY is more than all lines set it to the last line
	if newY >= len(v.lines) {
		newY = len(v.lines) - 1
	}
	if newY < 0 {
		newY = 0
	}

	line := v.lines[newY]

	// If newX is more than the line width go to the next line if possible
	// Otherwhise do nothing
	if newX > len(line) {
		if dy == 0 && newY+1 < len(v.lines) {
			newY++
			// line = v.lines[newY] // Uncomment if adding code that uses line
			newX = 0
		} else {
			newX = len(line)
		}
	}

	// If newX is less than 0 try goint to the previous line's last char
	if newX < 0 {
		if newY > 0 {
			newY--
			line = v.lines[newY]
			newX = len(line)
		} else {
			newX = 0
		}
	}

	maxX, maxY := v.Size()
	newXOnScreen, newYOnScreen, _ := v.linesPosOnScreen(newX, newY)

	// Set the view offset
	if newYOnScreen > v.oy+maxY-1 {
		v.oy = newYOnScreen - maxY + 1
	}
	if newYOnScreen < v.oy {
		v.oy = newYOnScreen
	}

	if !v.Wrap {
		if newXOnScreen > v.ox+maxX-1 {
			v.ox = newXOnScreen - maxX + 1
		}
		// Size of the line preview when moving to the left edge.
		// This should help to display hidden text of the line when
		// wrapping is off and moving towards beginning of the line.
		// 2 is currently set as the max length of characters which are visible.
		prevSize := 0 // this is default, no preview (used when hitting the beginnig)
		if maxX > 2 && newXOnScreen-2 >= 0 {
			prevSize = 2
		} else if maxX > 1 && newXOnScreen-1 >= 0 {
			prevSize = 1
		}
		if newXOnScreen-prevSize < v.ox {
			v.ox = newXOnScreen - prevSize
		}
	}

	v.cx, v.cy = newX, newY
}

// writeRune writes a rune into the view's internal buffer, at the
// position corresponding to the point (x, y). The length of the internal
// buffer is increased if the point is out of bounds. Overwrite mode is
// governed by the value of View.overwrite.
func (v *View) writeRune(x, y int, ch rune) error {
	v.tainted = true

	if x < 0 || y < 0 {
		return errors.New("invalid point")
	}

	if y >= len(v.lines) {
		newLines := make([][]cell, y-len(v.lines)+1)
		v.lines = append(v.lines, newLines...)
	}

	line := v.lines[y]
	lineLen := len(line)

	var toInsert []cell
	if x >= lineLen {
		toInsert = make([]cell, x-lineLen+1)
	} else if !v.Overwrite {
		toInsert = make([]cell, 1)
	}
	v.lines[y] = append(v.lines[y], toInsert...)

	if !v.Overwrite || (v.Overwrite && x+1 >= lineLen) {
		copy(v.lines[y][x+1:], v.lines[y][x:])
	}

	v.lines[y][x] = cell{
		fgColor: v.FgColor,
		bgColor: v.BgColor,
		chr:     ch,
	}

	return nil
}

// deleteRune removes a rune from the view's internal buffer, at the
// position corresponding to the point (x, y).
// returns error if invalid point is specified.
func (v *View) deleteRune(x, y int) error {
	v.tainted = true

	if x < 0 || y < 0 || y >= len(v.lines) || x >= len(v.lines[y]) {
		return errors.New("invalid point")
	}

	v.lines[y] = append(v.lines[y][:x], v.lines[y][x+1:]...)
	return nil
}

// mergeLines merges the lines "y" and "y+1" if possible.
func (v *View) mergeLines(y int) error {
	v.tainted = true

	if y < 0 || y >= len(v.lines) {
		return errors.New("invalid point")
	}

	if y+1 < len(v.lines) { // If we are already on the last line this would panic
		v.lines[y] = append(v.lines[y], v.lines[y+1]...)
		v.lines = append(v.lines[:y+1], v.lines[y+2:]...)
	}
	return nil
}

// breakLine breaks a line of the internal buffer at the position corresponding
// to the point (x, y).
func (v *View) breakLine(x, y int) error {
	v.tainted = true

	if y < 0 || y >= len(v.lines) {
		return errors.New("invalid point")
	}

	var left, right []cell
	if x < len(v.lines[y]) { // break line
		left = make([]cell, len(v.lines[y][:x]))
		copy(left, v.lines[y][:x])
		right = make([]cell, len(v.lines[y][x:]))
		copy(right, v.lines[y][x:])
	} else { // new empty line
		left = v.lines[y]
	}

	lines := make([][]cell, len(v.lines)+1)
	lines[y] = left
	lines[y+1] = right
	copy(lines, v.lines[:y])
	copy(lines[y+2:], v.lines[y+1:])
	v.lines = lines
	return nil
}
