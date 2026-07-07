package kaya

type State struct {
	Stress     int
	Trust      int
	Fear       int
	Pain       int
	Exhaustion int
	Injured    bool
	HasDoubt   bool
}

func (s State) Willingness() int {
	return s.Trust - s.Stress - s.Fear - s.Pain - s.Exhaustion
}

func (s State) LikelyToRefuse() bool {
	return s.Willingness() < -20
}
