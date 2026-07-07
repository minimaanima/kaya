package world

func MatchesTarget(target string, name string, aliases []string) bool {
	target = normalizeTarget(target)
	if target == "" {
		return false
	}
	return matchesName(target, name, aliases)
}
