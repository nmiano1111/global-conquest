package risk

import "fmt"

type Territory string
type Continent string
type Symbol string

const (
	Infantry  Symbol = "infantry"
	Cavalry   Symbol = "cavalry"
	Artillery Symbol = "artillery"
	Wild      Symbol = "wild"
)

type Card struct {
	Territory Territory
	Symbol    Symbol
}

type ContinentInfo struct {
	Bonus       int
	Territories []Territory
}

type Board struct {
	Continents map[Continent]ContinentInfo
	Adjacent   map[Territory]map[Territory]struct{}
	Order      []Territory
}

func ClassicBoard() Board {
	continents := map[Continent]ContinentInfo{
		"north_america": {
			Bonus: 5,
			Territories: []Territory{
				"Alaska", "Northwest Territory", "Greenland", "Alberta", "Ontario",
				"Quebec", "Western United States", "Eastern United States", "Central America",
			},
		},
		"south_america": {
			Bonus:       2,
			Territories: []Territory{"Venezuela", "Peru", "Brazil", "Argentina"},
		},
		"europe": {
			Bonus: 5,
			Territories: []Territory{
				"Iceland", "Scandinavia", "Ukraine", "Great Britain",
				"Northern Europe", "Western Europe", "Southern Europe",
			},
		},
		"africa": {
			Bonus: 3,
			Territories: []Territory{
				"North Africa", "Egypt", "East Africa", "Congo", "South Africa", "Madagascar",
			},
		},
		"asia": {
			Bonus: 7,
			Territories: []Territory{
				"Ural", "Siberia", "Yakutsk", "Kamchatka", "Irkutsk", "Mongolia", "Japan",
				"Afghanistan", "Middle East", "India", "Siam", "China",
			},
		},
		"australia": {
			Bonus:       2,
			Territories: []Territory{"Indonesia", "New Guinea", "Western Australia", "Eastern Australia"},
		},
	}

	order := []Territory{
		"Alaska", "Northwest Territory", "Greenland", "Alberta", "Ontario",
		"Quebec", "Western United States", "Eastern United States", "Central America",
		"Venezuela", "Peru", "Brazil", "Argentina",
		"Iceland", "Scandinavia", "Ukraine", "Great Britain", "Northern Europe", "Western Europe", "Southern Europe",
		"North Africa", "Egypt", "East Africa", "Congo", "South Africa", "Madagascar",
		"Ural", "Siberia", "Yakutsk", "Kamchatka", "Irkutsk", "Mongolia", "Japan", "Afghanistan", "Middle East", "India", "Siam", "China",
		"Indonesia", "New Guinea", "Western Australia", "Eastern Australia",
	}

	adj := map[Territory][]Territory{
		"Alaska":                {"Northwest Territory", "Alberta", "Kamchatka"},
		"Northwest Territory":   {"Alaska", "Alberta", "Ontario", "Greenland"},
		"Greenland":             {"Northwest Territory", "Ontario", "Quebec", "Iceland"},
		"Alberta":               {"Alaska", "Northwest Territory", "Ontario", "Western United States"},
		"Ontario":               {"Northwest Territory", "Greenland", "Quebec", "Eastern United States", "Western United States", "Alberta"},
		"Quebec":                {"Ontario", "Greenland", "Eastern United States"},
		"Western United States": {"Alberta", "Ontario", "Eastern United States", "Central America"},
		"Eastern United States": {"Western United States", "Ontario", "Quebec", "Central America"},
		"Central America":       {"Western United States", "Eastern United States", "Venezuela"},

		"Venezuela": {"Central America", "Brazil", "Peru"},
		"Peru":      {"Venezuela", "Brazil", "Argentina"},
		"Brazil":    {"Venezuela", "Peru", "Argentina", "North Africa"},
		"Argentina": {"Peru", "Brazil"},

		"Iceland":         {"Greenland", "Great Britain", "Scandinavia"},
		"Scandinavia":     {"Iceland", "Great Britain", "Northern Europe", "Ukraine"},
		"Ukraine":         {"Scandinavia", "Northern Europe", "Southern Europe", "Middle East", "Afghanistan", "Ural"},
		"Great Britain":   {"Iceland", "Scandinavia", "Northern Europe", "Western Europe"},
		"Northern Europe": {"Great Britain", "Scandinavia", "Ukraine", "Southern Europe", "Western Europe"},
		"Western Europe":  {"Great Britain", "Northern Europe", "Southern Europe", "North Africa"},
		"Southern Europe": {"Western Europe", "Northern Europe", "Ukraine", "Middle East", "Egypt", "North Africa"},

		"North Africa": {"Brazil", "Western Europe", "Southern Europe", "Egypt", "East Africa", "Congo"},
		"Egypt":        {"North Africa", "Southern Europe", "Middle East", "East Africa"},
		"East Africa":  {"Egypt", "North Africa", "Congo", "South Africa", "Madagascar", "Middle East"},
		"Congo":        {"North Africa", "East Africa", "South Africa"},
		"South Africa": {"Congo", "East Africa", "Madagascar"},
		"Madagascar":   {"South Africa", "East Africa"},

		"Ural":        {"Ukraine", "Siberia", "China", "Afghanistan"},
		"Siberia":     {"Ural", "Yakutsk", "Irkutsk", "Mongolia", "China"},
		"Yakutsk":     {"Siberia", "Irkutsk", "Kamchatka"},
		"Kamchatka":   {"Yakutsk", "Irkutsk", "Mongolia", "Japan", "Alaska"},
		"Irkutsk":     {"Siberia", "Yakutsk", "Kamchatka", "Mongolia"},
		"Mongolia":    {"Siberia", "Irkutsk", "Kamchatka", "Japan", "China"},
		"Japan":       {"Kamchatka", "Mongolia"},
		"Afghanistan": {"Ukraine", "Ural", "China", "India", "Middle East"},
		"Middle East": {"Ukraine", "Southern Europe", "Egypt", "East Africa", "India", "Afghanistan"},
		"India":       {"Middle East", "Afghanistan", "China", "Siam"},
		"Siam":        {"India", "China", "Indonesia"},
		"China":       {"Mongolia", "Siberia", "Ural", "Afghanistan", "India", "Siam"},

		"Indonesia":         {"Siam", "New Guinea", "Western Australia"},
		"New Guinea":        {"Indonesia", "Western Australia", "Eastern Australia"},
		"Western Australia": {"Indonesia", "New Guinea", "Eastern Australia"},
		"Eastern Australia": {"New Guinea", "Western Australia"},
	}

	a := make(map[Territory]map[Territory]struct{}, len(adj))
	for t, ns := range adj {
		a[t] = map[Territory]struct{}{}
		for _, n := range ns {
			a[t][n] = struct{}{}
		}
	}

	return Board{
		Continents: continents,
		Adjacent:   a,
		Order:      order,
	}
}

func (b Board) IsAdjacent(a, c Territory) bool {
	_, ok := b.Adjacent[a][c]
	return ok
}

func (b Board) Validate() error {
	for _, t := range b.Order {
		if _, ok := b.Adjacent[t]; !ok {
			return fmt.Errorf("missing adjacency for territory %q", t)
		}
	}
	for a, ns := range b.Adjacent {
		for c := range ns {
			if _, ok := b.Adjacent[c][a]; !ok {
				return fmt.Errorf("adjacency not symmetric: %q -> %q", a, c)
			}
		}
	}
	return nil
}

func ClassicDeck(order []Territory) []Card {
	syms := []Symbol{Infantry, Cavalry, Artillery}
	out := make([]Card, 0, len(order)+2)
	for i, t := range order {
		out = append(out, Card{
			Territory: t,
			Symbol:    syms[i%len(syms)],
		})
	}
	out = append(out, Card{Symbol: Wild}, Card{Symbol: Wild})
	return out
}
