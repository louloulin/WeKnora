package handler

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Tencent/WeKnora/internal/types"
)

// FeaturesHandler serves GET /api/v1/features. It reads runtime
// feature flags from environment variables at request time so no
// configuration is required at startup and a process restart is not
// needed for an env-only toggle to take effect. Build #2b wires
// WEKNORA_FEATURE_WIKI_WYSIWYG here; subsequent Builds may add
// additional flag fields without changing the route shape.
type FeaturesHandler struct{}

// NewFeaturesHandler is a no-op constructor kept for symmetry with
// other handlers in the package.
func NewFeaturesHandler() *FeaturesHandler { return &FeaturesHandler{} }

// GetFeatures godoc
// @Summary      获取运行时 feature flag
// @Description  返回当前进程生效的运行时 flag map；前端按 flag 决定 UI 行为。
//  字段 wiki_wysiwyg 由环境变量 WEKNORA_FEATURE_WIKI_WYSIWYG 控制（接受 true/1/yes，其余视为 false）。
// @Tags         系统
// @Produce      json
// @Success      200  {object}  map[string]interface{}  "标准 code/msg/data 包装，data 为 types.FeaturesResponse"
// @Router       /features [get]
func (h *FeaturesHandler) GetFeatures(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"code": 0,
		"msg":  "success",
		"data": types.FeaturesResponse{
			Flags: types.FeaturesFlags{
				WikiWysiwyg: parseBoolEnv("WEKNORA_FEATURE_WIKI_WYSIWYG"),
			},
		},
	})
}

// parseBoolEnv returns true iff the env var is set to a value that
// (after trim + lowercase) equals "true", "1", or "yes". Empty /
// unset / "false" / "0" / "no" / anything else all return false.
//
// Intentionally not case-folded against the full set of Go strconv
// truthy values — the surface stays small so an operator reading
// the env var can predict the outcome without consulting docs.
func parseBoolEnv(name string) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return false
	}
	switch strings.ToLower(v) {
	case "true", "1", "yes":
		return true
	default:
		return false
	}
}