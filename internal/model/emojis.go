package model

import (
	_ "embed"
	"encoding/json"
)

//go:embed emojis.json
var emojiJSON []byte
var reactionEmojis = func() map[string]bool {
	var groups [][]string
	if err := json.Unmarshal(emojiJSON, &groups); err != nil {
		panic(err)
	}
	allowed := map[string]bool{"": true} // Removal also works for unsupported incoming emoji.
	for _, g := range groups {
		for _, e := range g {
			allowed[e] = true
		}
	}
	return allowed
}()

func ValidReaction(emoji string) bool { return reactionEmojis[emoji] }
