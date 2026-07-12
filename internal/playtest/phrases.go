package playtest

type PhraseBank struct {
	Awareness      []string
	Search         []string
	TakeFlashlight []string
	MoveEast       []string
	LightOn        []string
	TakeKey        []string
	Unlock         []string
	MoveNorth      []string
}

var PrototypePhrases = PhraseBank{
	Awareness:      []string{"look around", "what do you see", "whats around you", "is there anything around you"},
	Search:         []string{"search the %s", "check the %s", "look inside the %s"},
	TakeFlashlight: []string{"take the flashlight", "grab the flashlight", "pick up the flashlight", "took the flashlight"},
	MoveEast:       []string{"go east", "move east", "head east", "walk east"},
	LightOn:        []string{"turn on the flashlight", "switch on the torch", "activate the light"},
	TakeKey:        []string{"take the key", "grab the brass key", "pick up the key", "took the key"},
	Unlock:         []string{"use the key on the emergency stairwell door", "try the key on the stairwell door"},
	MoveNorth:      []string{"go north", "move north", "head north"},
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
