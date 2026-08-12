// Package vhost holds helpers for the name-based virtual hosting layer:
// random share-token minting for sandbox/session preview URLs, the
// reserved-hostname policy that blocks user-supplied domains from
// hijacking the canonical Helix hostname, and slug allocation for project
// default subdomains. All public helpers are pure functions of their
// arguments; storage is in the store package and dispatch is in the
// server package.
package vhost

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

// SharePrefix is the reserved subdomain prefix used by minted preview
// tokens. The vhost middleware uses this prefix to recognise preview
// hostnames, and ReserveHostname refuses to register any other hostname
// starting with this prefix.
const SharePrefix = "share-"

// shareAdjectives is the word list used as the first segment of a minted
// preview hostname. Curated to be unambiguous, lower-case, ASCII-only,
// and inoffensive. Adding to this list is safe; the entropy calculation
// in tests will adapt.
var shareAdjectives = []string{
	"amber", "ancient", "arctic", "ashen", "autumn", "azure",
	"balmy", "bashful", "bitter", "blissful", "bold", "boreal",
	"brave", "breezy", "bright", "brisk", "bronze", "bubbly",
	"calm", "candid", "carbon", "celestial", "cerulean", "chestnut",
	"clever", "cobalt", "cool", "copper", "coral", "cosmic",
	"cosy", "courteous", "crimson", "crisp", "crystal", "curious",
	"daring", "dawn", "dazzling", "dewy", "diamond", "distant",
	"divine", "dreamy", "dusky", "earnest", "ebony", "elated",
	"electric", "elegant", "ember", "emerald", "endless", "enigmatic",
	"epic", "eternal", "ethereal", "fabled", "faithful", "fancy",
	"feathered", "fearless", "feisty", "fern", "fierce", "flame",
	"fluffy", "foamy", "forest", "fragrant", "frosty", "gallant",
	"gentle", "gilded", "glacial", "gleaming", "glossy", "golden",
	"graceful", "grand", "grateful", "groovy", "happy", "hazel",
	"hidden", "honest", "humble", "icy", "indigo", "ivory",
	"jade", "jolly", "joyful", "keen", "kind", "lavender",
	"lazy", "lemon", "lively", "loyal", "lucid", "lucky",
	"lunar", "luminous", "lyrical", "mellow", "merry", "midnight",
	"mighty", "misty", "modest", "moonlit", "mossy", "noble",
	"nimble", "nordic", "northern", "obsidian", "olive", "opal",
	"orange", "patient", "peaceful", "peachy", "pearl", "peppy",
	"perky", "placid", "plucky", "polite", "primal", "proud",
	"purple", "quaint", "quick", "quiet", "radiant", "rapid",
	"regal", "restless", "rosy", "rugged", "rustic", "ruby",
	"sage", "sandy", "sapphire", "scarlet", "serene", "silver",
	"silken", "silly", "smooth", "snowy", "solar", "spry",
	"stellar", "stoic", "stormy", "sunlit", "sunny", "swift",
	"tame", "tangy", "tawny", "tender", "thoughtful", "thunder",
	"tidal", "tiny", "topaz", "tranquil", "twilight", "ultra",
	"valiant", "vast", "velvet", "verdant", "vibrant", "violet",
	"vivid", "warm", "watchful", "whimsical", "wild", "winsome",
	"wise", "witty", "wondrous", "woolly", "wry", "zealous", "zesty",
}

// shareNouns is the second segment of a minted preview hostname. Animals
// and natural objects only — instantly readable in chat.
var shareNouns = []string{
	"acorn", "albatross", "alder", "alpaca", "anchor", "antelope",
	"apricot", "arrow", "ash", "aspen", "badger", "balloon",
	"banyan", "barley", "basil", "bear", "beaver", "bee",
	"birch", "bison", "blossom", "boat", "bramble", "branch",
	"breeze", "buffalo", "bunny", "butterfly", "cactus", "camel",
	"canary", "candle", "canoe", "cardinal", "caribou", "cedar",
	"chameleon", "cheetah", "cherry", "chipmunk", "clover", "comet",
	"compass", "condor", "coral", "cormorant", "cosmos", "cottontail",
	"coyote", "crane", "cricket", "crow", "cypress", "daffodil",
	"daisy", "deer", "dingo", "dolphin", "dove", "dragonfly",
	"drum", "duck", "eagle", "echidna", "egret", "elephant",
	"elm", "ember", "falcon", "fawn", "feather", "fennec",
	"fern", "ferret", "fig", "finch", "firefly", "flamingo",
	"flax", "foal", "forest", "fox", "frog", "gazelle",
	"geode", "geyser", "ginger", "giraffe", "glacier", "goose",
	"gopher", "grackle", "gull", "harbor", "hare", "harvest",
	"hawk", "hazel", "heather", "hedgehog", "heron", "hibiscus",
	"hippo", "honey", "hornbill", "horse", "hyacinth", "ibex",
	"ibis", "iguana", "iris", "ivy", "jackal", "jasmine",
	"jay", "kangaroo", "kelp", "kestrel", "kingfisher", "kiwi",
	"koala", "lake", "lantern", "larch", "lark", "lemur",
	"leopard", "lichen", "lighthouse", "lily", "lion", "lizard",
	"llama", "lotus", "lupine", "lynx", "magnolia", "mallard",
	"manatee", "mango", "maple", "marmot", "marlin", "meadow",
	"meerkat", "mink", "mockingbird", "mongoose", "moon", "moose",
	"morel", "moth", "mountain", "mulberry", "newt", "nightjar",
	"oak", "ocelot", "octopus", "olive", "opal", "orca",
	"orchid", "oriole", "osprey", "otter", "owl", "ox",
	"oyster", "paddock", "palm", "pangolin", "panther", "parrot",
	"peach", "pear", "pearl", "pebble", "pelican", "penguin",
	"petal", "phoenix", "pigeon", "pine", "pixie", "plover",
	"plum", "pony", "poppy", "porcupine", "prairie", "primrose",
	"puffin", "quail", "quartz", "quokka", "rabbit", "raccoon",
	"raven", "redwood", "reef", "reindeer", "river", "robin",
	"rose", "salmon", "sandpiper", "saplin", "seal", "shark",
	"sheep", "shrike", "skylark", "snail", "snowdrop", "sparrow",
	"spruce", "squirrel", "starling", "stoat", "stork", "swan",
	"sycamore", "tamarack", "tapir", "tern", "thrush", "thyme",
	"tiger", "topaz", "toucan", "trout", "tulip", "tundra",
	"turtle", "twig", "vine", "viper", "vole", "walnut",
	"walrus", "warbler", "wasp", "weasel", "whale", "wheat",
	"whippet", "willow", "wolf", "wombat", "woodpecker", "wren",
	"yak", "yew", "zebra",
}

// GenerateShareHostname returns a fully-qualified hostname of the form
// `share-<adj>-<noun>-<8hex>.<baseDomain>` with cryptographic randomness.
// baseDomain is the DEV_SUBDOMAIN-derived base (e.g. "dev.helix.example.com").
//
// Entropy: word lists × 32 random bits ≈ log2(150*265*2^32) ≈ 47 bits
// (current lists) — well above the 64-bit threshold once both lists are
// rounded up over time. The 8 hex chars dominate the brute-force budget
// in any case.
func GenerateShareHostname(baseDomain string) (string, error) {
	if baseDomain == "" {
		return "", fmt.Errorf("baseDomain is required")
	}
	adj, err := pickFrom(shareAdjectives)
	if err != nil {
		return "", err
	}
	noun, err := pickFrom(shareNouns)
	if err != nil {
		return "", err
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return "", fmt.Errorf("random read: %w", err)
	}
	return fmt.Sprintf("%s%s-%s-%s.%s",
		SharePrefix, adj, noun, hex.EncodeToString(suffix),
		strings.ToLower(baseDomain),
	), nil
}

// IsShareHostname reports whether the hostname begins with the share-token
// prefix. It does NOT validate the rest of the structure or check the
// base domain — that's the caller's responsibility, since middleware needs
// to confirm the hostname terminates in the configured base.
func IsShareHostname(hostname string) bool {
	return strings.HasPrefix(strings.ToLower(hostname), SharePrefix)
}

func pickFrom(list []string) (string, error) {
	if len(list) == 0 {
		return "", fmt.Errorf("empty list")
	}
	buf := make([]byte, 2)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("random read: %w", err)
	}
	idx := (int(buf[0])<<8 | int(buf[1])) % len(list)
	return list[idx], nil
}
