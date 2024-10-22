package ebiplayer

type vKeyMode struct {
	display  string
	remove   int
	add      string
	switchTo int
}

type vKey struct {
	modes []vKeyMode
}

func (k vKey) modeAt(m int) vKeyMode {
	for m >= len(k.modes) {
		i := 1
		for i <= m {
			i <<= 1
		}
		m -= i >> 1
	}
	return k.modes[m]
}

func (k vKeyMode) applyTo(s string, m int) (string, int) {
	if k.remove > 0 {
		r := []rune(s)
		if len(r) < k.remove {
			return "", m
		}
		return string(r[0 : len(r)-k.remove]), m
	}
	if k.add != "" {
		return s + k.add, m
	}
	return s, k.switchTo
}
