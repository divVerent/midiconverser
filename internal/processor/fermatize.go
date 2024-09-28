package processor

type tickFermata struct {
	tick   int64
	extend int64
	rest   int64

	// Values computed from the inputs.
	holdTick    int64 // Last tick where all notes are held.
	releaseTick int64 // First tick with a note after the fermata; -1 indicates till end.
}

func maybeFermatize(c cut, fermataTick []tickFermata, doit bool) []cut {
	if doit {
		return fermatize(c, fermataTick)
	} else {
		return []cut{c}
	}
}

func fermatize(c cut, fermataTick []tickFermata) []cut {
	var result []cut
	for _, tf := range fermataTick {
		if tf.holdTick >= c.Begin && tf.holdTick < c.End && tf.releaseTick < c.End {
			result = append(result,
				cut{
					RestBefore: c.RestBefore,
					Begin:      c.Begin,
					End:        tf.holdTick,
					RestAfter:  tf.extend,
					DirtyBegin: c.DirtyBegin,
					DirtyEnd:   true,
				})
			if tf.releaseTick >= 0 {
				result = append(result,
					cut{
						RestBefore:       0,
						Begin:            tf.holdTick,
						End:              tf.releaseTick,
						RestAfter:        tf.rest,
						DirtyBegin:       true,
						DirtyEnd:         false,
						AllNotesOffAtEnd: true,
					})
				c.Begin = tf.releaseTick
				c.RestBefore = 0
				c.DirtyBegin = false
			} else {
				c.Begin = tf.holdTick
				c.RestBefore = 0
				c.DirtyBegin = true
			}
		}
	}
	return append(result, c)
}
