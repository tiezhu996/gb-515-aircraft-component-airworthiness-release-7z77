package handler

import (
	"net/http"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/dto"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/middleware"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/service"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/util"
	"github.com/gin-gonic/gin"
)

type PartAssemblyHandler struct {
	service service.PartAssemblyService
}

func NewPartAssemblyHandler(s service.PartAssemblyService) *PartAssemblyHandler {
	return &PartAssemblyHandler{service: s}
}

// Register keeps a dedicated /part-assembly prefix so the static routes never
// collide with /parts/:id's wildcard segment.
func (h *PartAssemblyHandler) Register(group *gin.RouterGroup) {
	resource := group.Group("/part-assembly")
	resource.GET("/:partId", h.view)
	resource.POST("", middleware.RequireMinimumRole("operator"), h.register)
	resource.DELETE("/:linkId", middleware.RequireMinimumRole("operator"), h.remove)
}

func (h *PartAssemblyHandler) view(c *gin.Context) {
	id, ok := parseIDParam(c, "partId")
	if !ok {
		return
	}
	view, err := h.service.GetPartAssembly(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, view)
}

func (h *PartAssemblyHandler) register(c *gin.Context) {
	var input dto.RegisterPartAssembly
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	view, err := h.service.Register(c.Request.Context(), input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.Created(c, view)
}

func (h *PartAssemblyHandler) remove(c *gin.Context) {
	id, ok := parseIDParam(c, "linkId")
	if !ok {
		return
	}
	if err := h.service.Remove(c.Request.Context(), id, actorFromContext(c), requestIDFromContext(c)); err != nil {
		handleError(c, err)
		return
	}
	util.NoContent(c)
}

func parseIDParam(c *gin.Context, name string) (uint, bool) {
	return parseRawID(c, c.Param(name))
}
