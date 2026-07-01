package gamename

import "math/rand/v2"

var adjectives = []string{
	"Angry", "Befuddled", "Blobby", "Bouncy", "Clumsy",
	"Confused", "Crusty", "Damp", "Dizzy", "Droopy",
	"Fluffy", "Flustered", "Gassy", "Gloomy", "Goofy",
	"Grumpy", "Hangry", "Itchy", "Jiggly", "Lanky",
	"Lumpy", "Mopey", "Mushy", "Nervous", "Pudgy",
	"Quirky", "Scruffy", "Shaky", "Sleepy", "Slimy",
	"Sloppy", "Smelly", "Sneaky", "Soggy", "Spooky",
	"Squiggly", "Stinky", "Sweaty", "Wiggly", "Wobbly",
}

var nouns = []string{
	"Badger", "Biscuit", "Blob", "Burrito", "Cabbage",
	"Caterpillar", "Cheese", "Chicken", "Clam", "Clog",
	"Dumpster", "Goblin", "Goose", "Hamster", "Hotdog",
	"Jellyfish", "Lasagna", "Lizard", "Meatball", "Muffin",
	"Nugget", "Onion", "Pigeon", "Platypus", "Possum",
	"Pretzel", "Raccoon", "Sandwich", "Sausage", "Sloth",
	"Snail", "Sock", "Spatula", "Squirrel", "Toad",
	"Turnip", "Waffle", "Walrus", "Weasel", "Zucchini",
}

// Generate returns a random "Adjective Noun" game name.
func Generate() string {
	adj := adjectives[rand.IntN(len(adjectives))]
	noun := nouns[rand.IntN(len(nouns))]
	return adj + " " + noun
}
