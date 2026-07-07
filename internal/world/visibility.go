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
