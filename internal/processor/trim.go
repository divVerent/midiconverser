package processor

func trim(c []cut) []cut {
	if len(c) == 0 {
		return c
	}
	result := append([]cut{}, c...)
	result[0].RestBefore = 0
	result[len(result)-1].RestAfter = 0
	for i := len(result) - 2; i >= 0; i-- {
		if !result[i].MayTrimRestAfter {
			break
		}
		result[i].RestAfter = 0
	}
	return result
}
