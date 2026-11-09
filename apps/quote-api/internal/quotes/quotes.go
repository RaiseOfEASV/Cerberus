// Package quotes holds the static, in-memory quote list quote-api serves.
package quotes

import "math/rand/v2"

// All is the full list of quotes served by GET /quote.
var All = []string{
	"The best way to get a bad law repealed is to enforce it strictly. - Abraham Lincoln",
	"Give me six hours to chop down a tree and I will spend the first four sharpening the axe. - Abraham Lincoln",
	"Three may keep a secret, if two of them are dead. - Benjamin Franklin",
	"Guard the gates well; what gets in is easier than what gets out. - old proverb",
	"Security is not a product, but a process. - Bruce Schneier",
	"Trust, but verify. - Russian proverb",
}

// Random returns a uniformly random entry from list, or "" if list is empty.
func Random(list []string) string {
	if len(list) == 0 {
		return ""
	}
	return list[rand.N(len(list))]
}
