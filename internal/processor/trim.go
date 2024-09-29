package processor

func trim(c []cut) []cut {
	// TODO also accept the MIDI file, and _really_ trim out silence parts as well?
	// I.e. remove fully silent segments at beginning or end.
	// Then cut first and last segment until first/last event.

	if len(c) == 0 {
		return c
	}
	result := append([]cut{}, c...)
	result[0].RestBefore = 0
	result[len(result)-1].RestAfter = 0
	return result
}
