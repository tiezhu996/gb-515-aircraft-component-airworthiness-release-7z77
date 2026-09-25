package handler

import (
	"errors"
	"net/http"

	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/repository"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/service"
	"github.com/blueship581/aircraft-component-airworthiness-release/backend/internal/util"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func handleError(c *gin.Context, err error) {
	var blocked *service.AssemblyBlockedError
	if errors.As(err, &blocked) {
		codes := make([]any, 0, len(blocked.Blocked))
		for _, item := range blocked.Blocked {
			codes = append(codes, gin.H{"id": item.ID, "code": item.Code, "name": item.Name, "status": item.Status, "reason": item.Reason, "level": item.Level})
		}
		c.AbortWithStatusJSON(http.StatusConflict, gin.H{
			"error": "assembly_release_blocked", "message": blocked.Error(),
			"blockedParts": codes,
		})
		return
	}
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		util.Fail(c, http.StatusNotFound, "not_found", "record was not found")
	case errors.Is(err, repository.ErrVersionConflict):
		util.Fail(c, http.StatusConflict, "version_conflict", "record changed; refresh and retry")
	case errors.Is(err, service.ErrForbidden):
		util.Fail(c, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, service.ErrLocked), errors.Is(err, service.ErrSeparationOfDuty):
		util.Fail(c, http.StatusConflict, "control_conflict", err.Error())
	case errors.Is(err, service.ErrAssemblyLinkNotFound), errors.Is(err, service.ErrAssemblyPartNotFound):
		util.Fail(c, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, service.ErrAssemblySelfReference), errors.Is(err, service.ErrAssemblyDuplicate),
		errors.Is(err, service.ErrAssemblyChildMounted), errors.Is(err, service.ErrAssemblyCycle):
		util.Fail(c, http.StatusConflict, "assembly_conflict", err.Error())
	case errors.Is(err, service.ErrInvalidTransition), errors.Is(err, service.ErrInvalidInput):
		util.Fail(c, http.StatusUnprocessableEntity, "business_rule", err.Error())
	default:
		_ = c.Error(err)
		util.Fail(c, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}
