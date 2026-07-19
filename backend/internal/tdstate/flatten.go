package tdstate

import (
	"fmt"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// Flatten converts Features into a fixed-order []float64 vector, for
// feeding into a linear value function or exporting to training data.
// Order: for each territory (in the same order Features.Territories was
// built, i.e. g.Board.Order) -- IsMine, ArmyFraction, Continent one-hot,
// IsContinentBorder, EnemyThreatFraction -- then the global block in
// GlobalFeatures' own field order (..., CardFraction, Defence, ...), with
// slice-valued fields expanded in place. FeatureNames(board) produces the
// matching names for this exact order, for labeling exported training
// data.
func (f Features) Flatten() []float64 {
	out := make([]float64, 0, f.width())
	for _, t := range f.Territories {
		out = append(out, boolToFloat(t.IsMine), t.ArmyFraction)
		out = append(out, boolsToFloats(t.Continent)...)
		out = append(out, boolToFloat(t.IsContinentBorder), t.EnemyThreatFraction)
	}
	g := f.Global
	out = append(out, g.MyArmyFraction, g.MyTerritoryFraction, g.MyIncomeFraction,
		g.StrongestEnemyArmyFraction, g.StrongestEnemyTerritoryFraction)
	out = append(out, g.ContinentArmyFraction...)
	out = append(out, g.CardFraction, g.Defence)
	out = append(out, boolsToFloats(g.Phase)...)
	out = append(out, boolToFloat(g.IsMyTurn))
	return out
}

func (f Features) width() int {
	if len(f.Territories) == 0 {
		return 0
	}
	perTerritory := 2 + len(f.Territories[0].Continent) + 2
	return len(f.Territories)*perTerritory + 5 + len(f.Global.ContinentArmyFraction) + 2 + len(f.Global.Phase) + 1
}

// FeatureNames produces the names matching Flatten()'s exact order, for
// board -- so exported training data can carry stable column labels
// without the Python side hardcoding continent names or count.
func FeatureNames(board risk.Board) []string {
	continents := sortedContinents(board)
	var names []string
	for _, t := range board.Order {
		names = append(names, fmt.Sprintf("territory_%s_is_mine", t))
		names = append(names, fmt.Sprintf("territory_%s_army_fraction", t))
		for _, c := range continents {
			names = append(names, fmt.Sprintf("territory_%s_continent_%s", t, c))
		}
		names = append(names, fmt.Sprintf("territory_%s_is_continent_border", t))
		names = append(names, fmt.Sprintf("territory_%s_enemy_threat_fraction", t))
	}
	names = append(names,
		"my_army_fraction", "my_territory_fraction", "my_income_fraction",
		"strongest_enemy_army_fraction", "strongest_enemy_territory_fraction",
	)
	for _, c := range continents {
		names = append(names, fmt.Sprintf("my_army_fraction_in_%s", c))
	}
	names = append(names, "card_fraction", "defence")
	for _, p := range allPhases {
		names = append(names, fmt.Sprintf("phase_%s", p))
	}
	names = append(names, "is_my_turn")
	return names
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func boolsToFloats(bs []bool) []float64 {
	out := make([]float64, len(bs))
	for i, b := range bs {
		out[i] = boolToFloat(b)
	}
	return out
}
