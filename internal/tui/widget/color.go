// gomuks - A terminal Matrix client written in Go.
// Copyright (C) 2020 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package widget

import (
	"hash/fnv"

	"github.com/gdamore/tcell/v2"

	"maunium.net/go/mautrix/id"
)

// The Monokai accent family (Remastered values from the Ghostty palette this
// fork lives in, plus two classics), replacing the old list of ~150 web color
// names, half of which were unreadable on a dark background.
var hashColors = []tcell.Color{
	tcell.NewHexColor(0xfd6883), // red
	tcell.NewHexColor(0xadda78), // green
	tcell.NewHexColor(0xf9cc6c), // yellow
	tcell.NewHexColor(0xf38d70), // orange
	tcell.NewHexColor(0xa8a9eb), // purple
	tcell.NewHexColor(0x85dacc), // cyan
	tcell.NewHexColor(0xf92672), // classic magenta
	tcell.NewHexColor(0xe6db74), // classic cream
}

// GetHashColor picks a stable Monokai accent for the given string (or user
// ID) based on its FNV-1a hash.
func GetHashColor(val interface{}) tcell.Color {
	var s string
	switch typed := val.(type) {
	case string:
		s = typed
	case *string:
		s = *typed
	case id.UserID:
		s = string(typed)
	default:
		return hashColors[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return hashColors[h.Sum32()%uint32(len(hashColors))]
}
