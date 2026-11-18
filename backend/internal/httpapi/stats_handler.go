package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/nmiano1111/global-conquest/backend/internal/service"
	"github.com/nmiano1111/global-conquest/backend/internal/store"
)

type statsService interface {
	Overview(ctx context.Context, f store.GameResultsFilter) (service.StatsOverview, error)
	Players(ctx context.Context, f store.GameResultsFilter) ([]service.PlayerSummary, error)
	PlayerDetail(ctx context.Context, key string, f store.GameResultsFilter) (service.PlayerDetail, error)
}

// SetStatsService wires in the service backing the admin player-statistics
// endpoints. Until set, those routes return 503.
func (h *Handler) SetStatsService(stats statsService) {
	h.stats = stats
}

// parseStatsFilter reads the shared stats query parameters:
// composition (all|human_only|mixed|bot_only), map (all|classic|custom),
// player_count (int), include_sandbox (bool), and from/to (YYYY-MM-DD or
// RFC3339; "to" as a bare date is inclusive of that whole day).
func parseStatsFilter(c *gin.Context) (store.GameResultsFilter, error) {
	f := store.GameResultsFilter{
		Composition: c.DefaultQuery("composition", store.CompositionAll),
		Map:         c.DefaultQuery("map", store.MapFilterAll),
	}
	switch f.Composition {
	case store.CompositionAll, store.CompositionHumanOnly, store.CompositionMixed, store.CompositionBotOnly:
	default:
		return f, fmt.Errorf("invalid composition %q", f.Composition)
	}
	switch f.Map {
	case store.MapFilterAll, store.MapFilterClassic, store.MapFilterCustom:
	default:
		return f, fmt.Errorf("invalid map %q", f.Map)
	}
	if v := c.Query("player_count"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, fmt.Errorf("invalid player_count %q", v)
		}
		f.PlayerCount = n
	}
	if v := c.Query("include_sandbox"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return f, fmt.Errorf("invalid include_sandbox %q", v)
		}
		f.IncludeSandboxed = b
	}
	for _, p := range []struct {
		name string
		dst  **time.Time
	}{{"from", &f.From}, {"to", &f.To}} {
		v := c.Query(p.name)
		if v == "" {
			continue
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			d, derr := time.Parse("2006-01-02", v)
			if derr != nil {
				return f, fmt.Errorf("invalid %s %q", p.name, v)
			}
			t = d
			if p.name == "to" {
				t = t.AddDate(0, 0, 1)
			}
		}
		*p.dst = &t
	}
	return f, nil
}

func (h *Handler) statsRequest(c *gin.Context) (store.GameResultsFilter, bool) {
	if h.stats == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "stats unavailable"})
		return store.GameResultsFilter{}, false
	}
	f, err := parseStatsFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return f, false
	}
	return f, true
}

// GetStatsOverview godoc
// @Summary      Site-wide game statistics
// @Description  Totals plus a per-day/per-week time series over completed games matching the filters. Admin only.
// @Tags         admin
// @Produce      json
// @Param        composition     query string false "all|human_only|mixed|bot_only"
// @Param        map             query string false "all|classic|custom"
// @Param        player_count    query int    false "Exact seat count"
// @Param        include_sandbox query bool   false "Include sandboxed games"
// @Param        from            query string false "YYYY-MM-DD or RFC3339"
// @Param        to              query string false "YYYY-MM-DD (inclusive) or RFC3339"
// @Success      200 {object} service.StatsOverview
// @Failure      400 {object} map[string]string
// @Router       /api/admin/stats/overview [get]
func (h *Handler) GetStatsOverview(c *gin.Context) {
	f, ok := h.statsRequest(c)
	if !ok {
		return
	}
	ov, err := h.stats.Overview(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load stats"})
		return
	}
	c.JSON(http.StatusOK, ov)
}

// ListStatsPlayers godoc
// @Summary      Per-player statistics
// @Description  One summary per human player and per bot strategy over completed games matching the filters. Admin only.
// @Tags         admin
// @Produce      json
// @Success      200 {array} service.PlayerSummary
// @Failure      400 {object} map[string]string
// @Router       /api/admin/stats/players [get]
func (h *Handler) ListStatsPlayers(c *gin.Context) {
	f, ok := h.statsRequest(c)
	if !ok {
		return
	}
	players, err := h.stats.Players(c.Request.Context(), f)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load stats"})
		return
	}
	c.JSON(http.StatusOK, players)
}

// GetStatsPlayer godoc
// @Summary      One player's statistics
// @Description  Summary, per-game time series, breakdowns, head-to-head, and history for a player key (user id, or bot:<strategy>). Admin only.
// @Tags         admin
// @Produce      json
// @Param        key path string true "Player key"
// @Success      200 {object} service.PlayerDetail
// @Failure      404 {object} map[string]string
// @Router       /api/admin/stats/players/{key} [get]
func (h *Handler) GetStatsPlayer(c *gin.Context) {
	f, ok := h.statsRequest(c)
	if !ok {
		return
	}
	d, err := h.stats.PlayerDetail(c.Request.Context(), c.Param("key"), f)
	if err != nil {
		if errors.Is(err, service.ErrStatsPlayerNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "no completed games for this player"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load stats"})
		return
	}
	c.JSON(http.StatusOK, d)
}
