package kaya

type Emotion string

const (
	EmotionCalm      Emotion = "calm"
	EmotionUneasy    Emotion = "uneasy"
	EmotionScared    Emotion = "scared"
	EmotionPanicked  Emotion = "panicked"
	EmotionAngry     Emotion = "angry"
	EmotionExhausted Emotion = "exhausted"
)

func (s State) DominantEmotion() Emotion {
	switch {
	case s.Stress >= 80 || s.Fear >= 80:
		return EmotionPanicked
	case s.Pain >= 60 || s.Exhaustion >= 70:
		return EmotionExhausted
	case s.Stress >= 50 || s.Fear >= 50:
		return EmotionScared
	case s.HasDoubt:
		return EmotionUneasy
	default:
		return EmotionCalm
	}
}
