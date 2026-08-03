package httpapi

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nmiano1111/global-conquest/backend/internal/mapgen"
	"github.com/nmiano1111/global-conquest/backend/internal/risk"
	"github.com/nmiano1111/global-conquest/backend/internal/service"
)

type createMapContinentReq struct {
	Name           string `json:"name" binding:"required"`
	Bonus          int    `json:"bonus" binding:"min=0"`
	TerritoryCount int    `json:"territory_count" binding:"required,min=1"`
}

type createMapBorderReq struct {
	A         string `json:"a" binding:"required"`
	B         string `json:"b" binding:"required"`
	Crossings int    `json:"crossings" binding:"required,min=1"`
}

type createMapReq struct {
	Name       string                  `json:"name" binding:"required"`
	Continents []createMapContinentReq `json:"continents" binding:"required,min=2,dive"`
	Borders    []createMapBorderReq    `json:"borders" binding:"dive"`
}

// CreateMap godoc
// @Summary      Create a custom map
// @Description  Generates and persists a new admin-authored map from a continent/border spec. Admin only.
// @Tags         maps
// @Accept       json
// @Produce      json
// @Param        request body createMapReq true "Create map request"
// @Success      201 {object} service.MapDetail
// @Failure      400 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/admin/maps [post]
func (h *Handler) CreateMap(c *gin.Context) {
	var req createMapReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	authUser, ok := getAuthUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	spec := mapgen.MapSpec{
		Name:       req.Name,
		Continents: make([]mapgen.ContinentSpec, len(req.Continents)),
		Borders:    make([]mapgen.ContinentBorder, len(req.Borders)),
	}
	for i, cont := range req.Continents {
		spec.Continents[i] = mapgen.ContinentSpec{Name: cont.Name, Bonus: cont.Bonus, TerritoryCount: cont.TerritoryCount}
	}
	for i, b := range req.Borders {
		spec.Borders[i] = mapgen.ContinentBorder{A: risk.Continent(b.A), B: risk.Continent(b.B), Crossings: b.Crossings}
	}

	m, err := h.maps.CreateMap(c.Request.Context(), authUser.ID, req.Name, spec)
	if err != nil {
		if errors.Is(err, service.ErrInvalidMapInput) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create map"})
		return
	}
	c.JSON(http.StatusCreated, m)
}

// ListMaps godoc
// @Summary      List custom maps
// @Description  Returns every stored custom map as a summary. Admin only.
// @Tags         maps
// @Produce      json
// @Success      200 {array} service.MapSummary
// @Failure      500 {object} map[string]string
// @Router       /api/admin/maps [get]
func (h *Handler) ListMaps(c *gin.Context) {
	maps, err := h.maps.ListMaps(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list maps"})
		return
	}
	c.JSON(http.StatusOK, maps)
}

// GetMap godoc
// @Summary      Get a custom map
// @Description  Returns a single custom map's full board and layout. Admin only.
// @Tags         maps
// @Produce      json
// @Param        id path string true "Map ID"
// @Success      200 {object} service.MapDetail
// @Failure      404 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/admin/maps/{id} [get]
func (h *Handler) GetMap(c *gin.Context) {
	mapID := c.Param("id")
	if mapID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "map id is required"})
		return
	}
	m, err := h.maps.GetMap(c.Request.Context(), mapID)
	if err != nil {
		if errors.Is(err, service.ErrMapNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "map not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch map"})
		return
	}
	c.JSON(http.StatusOK, m)
}

// DeleteMap godoc
// @Summary      Delete a custom map
// @Description  Permanently deletes a custom map. Fails if any game still references it. Admin only.
// @Tags         maps
// @Param        id path string true "Map ID"
// @Success      204
// @Failure      404 {object} map[string]string
// @Failure      409 {object} map[string]string
// @Failure      500 {object} map[string]string
// @Router       /api/admin/maps/{id} [delete]
func (h *Handler) DeleteMap(c *gin.Context) {
	mapID := c.Param("id")
	if mapID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "map id is required"})
		return
	}
	if err := h.maps.DeleteMap(c.Request.Context(), mapID); err != nil {
		switch {
		case errors.Is(err, service.ErrMapNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "map not found"})
		case errors.Is(err, service.ErrMapInUse):
			c.JSON(http.StatusConflict, gin.H{"error": "map is in use by one or more games"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete map"})
		}
		return
	}
	c.Status(http.StatusNoContent)
}
