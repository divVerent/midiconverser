//go:build ios

package ebiplayer

var vKeys = func() [][]vKey {
	shift := vKey{
		modes: []vKeyMode{
			{
				display:  "AB",
				switchTo: 1,
			},
			{
				display:  "ab",
				switchTo: 0,
			},
			{
				display:  "#+",
				switchTo: 3,
			},
			{
				display:  "12",
				switchTo: 2,
			},
		},
	}
	backspace := vKey{
		modes: []vKeyMode{
			{
				display: "<-",
				remove:  1,
			},
		},
	}
	alt := vKey{
		modes: []vKeyMode{
			{
				display:  "12",
				switchTo: 2,
			},
			{
				display:  "12",
				switchTo: 2,
			},
			{
				display:  "ab",
				switchTo: 0,
			},
			{
				display:  "ab",
				switchTo: 0,
			},
		},
	}
	space := vKey{
		modes: []vKeyMode{
			{
				display: "SPACE",
				add:     " ",
			},
		},
	}
	n := func(chs string) vKey {
		k := vKey{}
		for _, ch := range chs {
			k.modes = append(k.modes, vKeyMode{
				display: string([]rune{ch}),
				add:     string([]rune{ch}),
			})
		}
		return k
	}
	// This is derived from the iPad keyboard on iOS 12.5.7.
	// Removed: anything non ASCII.
	// Added: missing backtick.
	// Now covers all ASCII printables.
	return [][]vKey{
		{
			n("qQ11"), n("wW22"), n("eE33"), n("rR44"), n("tT55"), n("yY66"), n("uU77"), n("iI88"), n("oO99"), n("pP00"),
		},
		{
			n("aA@@"), n("sS##"), n("dD$`"), n("fF&_"), n("gG*^"), n("hH(["), n("jJ)]"), n("kK'{"), n("lL\"}"), backspace,
		},
		{
			shift, n("zZ%%"), n("xX-|"), n("cC+~"), n("vV=="), n("bB/\\"), n("nN;<"), n("mM:>"), n(",!"), n(".?"),
		},
		{
			alt, space, alt,
		},
	}
}()
