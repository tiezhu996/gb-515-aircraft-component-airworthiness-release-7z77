package handler

import (
	"net/http"
	"strconv"

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

func (h *PartAssemblyHandler) Register(group *gin.RouterGroup) {
	resource := group.Group("/parts/:id/assembly")
	resource.GET("", h.view)
	resource.POST("", middleware.RequireMinimumRole("operator"), h.register)
	resource.DELETE("/:childId", middleware.RequireMinimumRole("operator"), h.remove)
}

func (h *PartAssemblyHandler) view(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	view, err := h.service.GetView(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, view)
}

func (h *PartAssemblyHandler) register(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var input dto.RegisterAssembly
	if err := c.ShouldBindJSON(&input); err != nil {
		util.Fail(c, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	// 路径里的组件部件优先，请求体里的 parentCode 仅作冗余校验。
	view, err := h.service.GetView(c.Request.Context(), id)
	if err != nil {
		handleError(c, err)
		return
	}
	input.ParentCode = view.Code
	result, err := h.service.Register(c.Request.Context(), input, actorFromContext(c), requestIDFromContext(c))
	if err != nil {
		handleError(c, err)
		return
	}
	util.Created(c, result)
}

func (h *PartAssemblyHandler) remove(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	childID, ok := parseChildID(c)
	if !ok {
		return
	}
	if err := h.service.Remove(c.Request.Context(), id, childID, actorFromContext(c), requestIDFromContext(c)); err != nil {
		handleError(c, err)
		return
	}
	util.NoContent(c)
}

func parseChildID(c *gin.Context) (uint, bool) {
	raw, err := strconv.ParseUint(c.Param("childId"), 10, 64)
	if err != nil || raw == 0 {
		util.Fail(c, http.StatusBadRequest, "invalid_id", "childId must be a positive integer")
		return 0, false
	}
	return uint(raw), true
}
