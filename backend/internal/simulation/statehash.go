package simulation

import (
	"encoding/binary"
	"hash"
	"hash/fnv"

	"github.com/nmiano1111/global-conquest/backend/internal/risk"
)

// StateFingerprint is a cheap, fixed-size identifier for a risk.Game's
// mutable state, used to detect a stuck or looping simulation without the
// cost of a full state comparison or JSON snapshot on every command. It
// is also what TraceFull records per command instead of a full state
// snapshot (see recorder.go).
type StateFingerprint uint64

// Fingerprint computes a StateFingerprint over g's mutable state: phase,
// current player, pending reinforcements, the per-turn flags, every
// territory's owner and army count, per-player card counts, and the
// occupy state. Territories are visited in g.Board.Order, not map
// iteration order, so the result is deterministic regardless of Go's
// randomized map iteration.
//
// TurnNumber and SetsTraded are deliberately excluded: both only ever
// increase, so including either would mean a genuine stuck loop -- the
// same board position recurring turn after turn -- never actually
// produces a repeated fingerprint, defeating the point of computing one.
func Fingerprint(g *risk.Game) StateFingerprint {
	h := fnv.New64a()

	writeString(h, string(g.Phase))
	writeInt(h, g.CurrentPlayer)
	writeInt(h, g.PendingReinforcements)
	writeBool(h, g.HasFortified)
	writeBool(h, g.ForcedCardTrade)
	writeBool(h, g.ConqueredThisTurn)

	for _, t := range g.Board.Order {
		ts := g.Territories[t]
		writeInt(h, ts.Owner)
		writeInt(h, ts.Armies)
	}

	for i := range g.Players {
		writeInt(h, len(g.Players[i].Cards))
	}

	if o := g.Occupy; o != nil {
		writeBool(h, true)
		writeString(h, string(o.From))
		writeString(h, string(o.To))
		writeInt(h, o.MinMove)
		writeInt(h, o.MaxMove)
	} else {
		writeBool(h, false)
	}

	return StateFingerprint(h.Sum64())
}

// writeString/writeInt/writeBool write a length-prefixed or fixed-width
// encoding of each value so that concatenating fields never produces an
// ambiguous byte stream (e.g. "ab"+"c" colliding with "a"+"bc").

func writeString(h hash.Hash64, s string) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(s)))
	h.Write(lenBuf[:])
	h.Write([]byte(s))
}

func writeInt(h hash.Hash64, n int) {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(int64(n)))
	h.Write(buf[:])
}

func writeBool(h hash.Hash64, b bool) {
	if b {
		h.Write([]byte{1})
		return
	}
	h.Write([]byte{0})
}
