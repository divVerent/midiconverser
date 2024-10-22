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
	return [][]vKey{
		{
			n("qQ1["), n("wW2]"), n("eE3{"), n("rR4}"), n("tT5#"), n("yY6%"), n("uU7^"), n("iI8*"), n("oO9+"), n("pP0="),
		},
		{
			n("aA-_"), n("sS/\\"), n("dD:|"), n("fF;~"), n("gG(<"), n("hH)>"), n("jJ$J"), n("kK%"), n("lL@"),
		},
		{
			shift, n("zZ"), n("xX.."), n("cC,,"), n("vV??"), n("bB!!"), n("nN''"), n("mM\"\""), backspace,
		},
		{
			alt, space, n("."),
		},
	}
}()
