package playtest

type PhraseBank struct {
	awareness      []string
	search         []string
	takeFlashlight []string
	moveEast       []string
	lightOn        []string
	takeKey        []string
	unlock         []string
	moveNorth      []string
}

var prototypePhrases = PhraseBank{
	awareness:      []string{"look around", "what do you see", "whats around you", "is there anything around you"},
	search:         []string{"search the %s", "check the %s", "look inside the %s"},
	takeFlashlight: []string{"take the flashlight", "grab the flashlight", "pick up the flashlight", "took the flashlight"},
	moveEast:       []string{"go east", "move east", "head east", "walk east"},
	lightOn:        []string{"turn on the flashlight", "switch on the torch", "activate the light"},
	takeKey:        []string{"take the key", "grab the brass key", "pick up the key", "took the key"},
	unlock:         []string{"use the key on the emergency stairwell door", "try the key on the stairwell door"},
	moveNorth:      []string{"go north", "move north", "head north"},
}

func PrototypePhrases() PhraseBank {
	return clonePhraseBank(prototypePhrases)
}

func clonePhraseBank(source PhraseBank) PhraseBank {
	return PhraseBank{
		awareness:      append([]string(nil), source.awareness...),
		search:         append([]string(nil), source.search...),
		takeFlashlight: append([]string(nil), source.takeFlashlight...),
		moveEast:       append([]string(nil), source.moveEast...),
		lightOn:        append([]string(nil), source.lightOn...),
		takeKey:        append([]string(nil), source.takeKey...),
		unlock:         append([]string(nil), source.unlock...),
		moveNorth:      append([]string(nil), source.moveNorth...),
	}
}

type splitMix64 struct {
	state uint64
}

func newSplitMix64(seed int64) splitMix64 {
	return splitMix64{state: uint64(seed)}
}

func (s *splitMix64) next() uint64 {
	s.state += 0x9e3779b97f4a7c15
	z := s.state
	z = (z ^ (z >> 30)) * 0xbf58476d1ce4e5b9
	z = (z ^ (z >> 27)) * 0x94d049bb133111eb
	return z ^ (z >> 31)
}

func (s *splitMix64) phrase(phrases []string) string {
	if len(phrases) == 0 {
		return ""
	}
	return phrases[s.next()%uint64(len(phrases))]
}
