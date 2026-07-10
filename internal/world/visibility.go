package world

type Visibility string

const (
	VisibilityLit        Visibility = "lit"
	VisibilityDim        Visibility = "dim"
	VisibilityDark       Visibility = "dark"
	VisibilityPitchBlack Visibility = "pitch_black"
)

func (v Visibility) RequiresLight() bool {
	return v == VisibilityDark || v == VisibilityPitchBlack
}

func CanSeeObject(room Room, object Object, activeLight bool) bool {
	if activeLight {
		return true
	}
	if room.Visibility == VisibilityPitchBlack {
		return false
	}
	if room.NeedsLight() {
		return !object.RequiresLight
	}
	return true
}
