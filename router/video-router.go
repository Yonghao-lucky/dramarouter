package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"

	"github.com/gin-gonic/gin"
)

// SetVideoRouter registers video routes that are not owned by the host
// protocol registry. POST /v1/videos, GET /v1/videos/:task_id and
// GET|HEAD /v1/videos/:task_id/content are registered by
// SetTaskPluginProtocolRouter.
func SetVideoRouter(router *gin.Engine) {
	videoSharedRouter := router.Group("/v1")
	videoSharedRouter.Use(middleware.RouteTag("relay"))
	videoSharedRouter.Use(middleware.TokenAuth())
	videoSharedRouter.Use(middleware.SystemPerformanceCheck())
	videoSharedRouter.POST(
		"/video/generations",
		middleware.PinTaskPluginEndpoint(),
		middleware.TaskPluginEndpointOnly(middleware.ModelRequestRateLimit()),
		middleware.PrepareTaskPluginEndpoint(),
		middleware.Distribute(),
		func(c *gin.Context) {
			controller.RelayTaskPluginEndpoint(c, controller.RelayTask)
		},
	)

	// Lets the authenticated dashboard choose between a direct redirect and
	// the same-origin content proxy before starting a download.
	videoContentInfoRouter := router.Group("/v1")
	videoContentInfoRouter.Use(middleware.RouteTag("relay"))
	videoContentInfoRouter.Use(middleware.TokenOrUserAuth())
	{
		videoContentInfoRouter.GET("/videos/:task_id/content-info", controller.GetVideoContentInfo)
	}

	videoV1WriteRouter := router.Group("/v1")
	videoV1WriteRouter.Use(middleware.RouteTag("relay"))
	videoV1WriteRouter.Use(middleware.TokenAuth(), middleware.Distribute())
	{
		videoV1WriteRouter.POST("/videos/:video_id/remix", controller.RelayTask)
	}

	// Task detail reads are also used by the authenticated dashboard. The
	// response builder still enforces owner access, with an administrator-only
	// cross-user lookup for the task log page.
	videoV1ReadRouter := router.Group("/v1")
	videoV1ReadRouter.Use(middleware.RouteTag("relay"))
	videoV1ReadRouter.Use(middleware.TokenOrUserAuth(), middleware.Distribute())
	{
		videoV1ReadRouter.GET("/video/generations/:task_id", controller.RelayTaskFetch)
	}

	// Fire Ark native async content-generation API. These routes deliberately
	// stay outside /v1 so only ChannelTypeVolcNative can serve them.
	volcNativeWriteRouter := router.Group("/api/v3")
	volcNativeWriteRouter.Use(middleware.RouteTag("relay"))
	volcNativeWriteRouter.Use(middleware.SystemPerformanceCheck())
	volcNativeWriteRouter.Use(middleware.TokenAuth())
	volcNativeWriteRouter.Use(middleware.ModelRequestRateLimit())
	{
		volcNativeWriteRouter.POST("/contents/generations/tasks", middleware.Distribute(), controller.RelayTask)
		volcNativeWriteRouter.DELETE("/contents/generations/tasks/:task_id", controller.RelayVolcNativeTaskDelete)
	}

	volcNativeReadRouter := router.Group("/api/v3")
	volcNativeReadRouter.Use(middleware.RouteTag("relay"))
	volcNativeReadRouter.Use(middleware.SystemPerformanceCheck())
	volcNativeReadRouter.Use(middleware.TokenOrUserAuth())
	{
		volcNativeReadRouter.GET("/contents/generations/tasks", controller.RelayVolcNativeTaskList)
		volcNativeReadRouter.GET("/contents/generations/tasks/:task_id", controller.RelayVolcNativeTaskFetch)
	}
}
