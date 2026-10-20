package controller

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	relaychannel "github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	storageService "github.com/QuantumNous/new-api/service/storage"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

type taskArtifactResponse struct {
	Key        string `json:"key"`
	Type       string `json:"type"`
	MimeType   string `json:"mime_type,omitempty"`
	ContentURL string `json:"content_url"`
}

var (
	taskArtifactKeyPattern           = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~-]{0,127}$`)
	errTaskArtifactPluginUnavailable = errors.New("task artifact plugin unavailable")
	errTaskArtifactPlugin            = errors.New("task artifact plugin error")
)

func GetTask(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("key"))
	if err != nil {
		videoProxyError(c, http.StatusInternalServerError, "server_error", "Failed to query task")
		return
	}
	if !exists {
		videoProxyError(c, http.StatusNotFound, "invalid_request_error", "Task not found")
		return
	}
	createdAt := task.CreatedAt
	if createdAt == 0 {
		createdAt = task.SubmitTime
	}
	failReason := task.FailReason
	if task.Status == model.TaskStatusSuccess && taskFailReasonIsLegacyResultURL(task.FailReason) {
		failReason = ""
	}
	c.JSON(http.StatusOK, gin.H{
		"task_id":     task.TaskID,
		"platform":    task.Platform,
		"status":      task.Status,
		"progress":    task.Progress,
		"fail_reason": failReason,
		"created_at":  createdAt,
		"finished_at": task.FinishTime,
	})
}

func GetTaskArtifacts(c *gin.Context) {
	task, exists, err := model.GetByTaskId(c.GetInt("id"), c.Param("key"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	writeTaskArtifacts(c, task, false)
}

func GetDashboardTaskArtifacts(c *gin.Context) {
	task, exists, err := getTaskForArtifactRequest(c, c.Param("task_id"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	writeTaskArtifacts(c, task, true)
}

func writeTaskArtifacts(c *gin.Context, task *model.Task, dashboard bool) {
	c.Header("Cache-Control", "private, no-store")
	artifacts, err := projectTaskArtifacts(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	items := make([]taskArtifactResponse, 0, len(artifacts))
	for _, artifact := range artifacts {
		contentURL, buildErr := service.BuildTaskArtifactContentURL(task.TaskID, artifact.Key)
		if buildErr != nil {
			writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_url_error", "Failed to build artifact content URL")
			return
		}
		items = append(items, taskArtifactResponse{
			Key:        artifact.Key,
			Type:       artifact.Type,
			MimeType:   artifact.MimeType,
			ContentURL: contentURL,
		})
	}
	response := gin.H{"task_id": task.TaskID, "artifacts": items}
	if legacyVideoAvailable(task) {
		legacyContentURL, buildErr := service.BuildTaskArtifactContentURL(task.TaskID, "video")
		if buildErr != nil {
			writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_url_error", "Failed to build artifact content URL")
			return
		}
		response["legacy_content_url"] = legacyContentURL
	}
	if dashboard {
		common.ApiSuccess(c, response)
		return
	}
	c.JSON(http.StatusOK, response)
}

func projectTaskArtifacts(task *model.Task) ([]relaychannel.TaskArtifact, error) {
	if task == nil || task.Status != model.TaskStatusSuccess || !taskHasPluginExecution(task) {
		return []relaychannel.TaskArtifact{}, nil
	}
	adaptor := relay.GetTaskAdaptor(task.Platform)
	if adaptor == nil {
		return nil, errTaskArtifactPluginUnavailable
	}
	provider, ok := adaptor.(relaychannel.TaskArtifactProvider)
	if !ok {
		return []relaychannel.TaskArtifact{}, nil
	}
	artifacts, err := provider.ListArtifacts(task)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errTaskArtifactPlugin, err)
	}
	return validateProjectedTaskArtifacts(artifacts)
}

func validateProjectedTaskArtifacts(artifacts []relaychannel.TaskArtifact) ([]relaychannel.TaskArtifact, error) {
	if len(artifacts) > 64 {
		return nil, fmt.Errorf("%w: too many artifacts", errTaskArtifactPlugin)
	}
	seen := make(map[string]struct{}, len(artifacts))
	for i := range artifacts {
		if artifacts[i].Key != strings.TrimSpace(artifacts[i].Key) ||
			artifacts[i].Type != strings.TrimSpace(artifacts[i].Type) {
			return nil, fmt.Errorf("%w: invalid artifact identity", errTaskArtifactPlugin)
		}
		if !taskArtifactKeyPattern.MatchString(artifacts[i].Key) {
			return nil, fmt.Errorf("%w: invalid artifact key", errTaskArtifactPlugin)
		}
		if _, exists := seen[artifacts[i].Key]; exists {
			return nil, fmt.Errorf("%w: duplicate artifact key", errTaskArtifactPlugin)
		}
		seen[artifacts[i].Key] = struct{}{}
		switch artifacts[i].Type {
		case "video", "audio", "image", "file":
		default:
			return nil, fmt.Errorf("%w: invalid artifact type", errTaskArtifactPlugin)
		}
		if len(artifacts[i].MimeType) > 255 || strings.ContainsAny(artifacts[i].MimeType, "\r\n") {
			return nil, fmt.Errorf("%w: invalid artifact mime type", errTaskArtifactPlugin)
		}
	}
	return artifacts, nil
}

func initTaskArtifactAdaptor(task *model.Task) (relaychannel.TaskAdaptor, error) {
	if task == nil || !taskHasPluginExecution(task) {
		return nil, errTaskArtifactPluginUnavailable
	}
	channelModel, err := model.CacheGetChannel(task.ChannelId)
	if err != nil {
		return nil, fmt.Errorf("%w: channel unavailable", errTaskArtifactPluginUnavailable)
	}
	adaptor := relay.GetTaskAdaptor(task.Platform)
	if adaptor == nil {
		return nil, errTaskArtifactPluginUnavailable
	}
	pluginKey := task.PrivateData.Key
	if pluginKey == "" {
		pluginKey = channelModel.Key
	}
	baseURL := channelModel.GetBaseURL()
	if baseURL == "" {
		baseURL = constant.GetChannelBaseURL(channelModel.Type)
	}
	adaptor.Init(&relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:    channelModel.Type,
			ChannelBaseUrl: baseURL,
			ApiKey:         pluginKey,
			ChannelSetting: channelModel.GetSetting(),
		},
	})
	return adaptor, nil
}

func taskHasPluginExecution(task *model.Task) bool {
	return task != nil &&
		task.PrivateData.Execution != nil &&
		task.PrivateData.Execution.TaskPlugin != nil &&
		strings.TrimSpace(task.PrivateData.Execution.TaskPlugin.Key) != ""
}

func legacyVideoAvailable(task *model.Task) bool {
	if task == nil || task.Status != model.TaskStatusSuccess ||
		taskHasPluginExecution(task) || task.Platform == constant.TaskPlatformSuno ||
		strings.TrimSpace(task.GetResultURL()) == "" {
		return false
	}
	switch constant.NormalizeTaskAction(task.Action) {
	case constant.TaskActionImageToVideo,
		constant.TaskActionTextToVideo,
		constant.TaskActionFirstTailToVideo,
		constant.TaskActionReferenceToVideo,
		constant.TaskActionRemix:
		return true
	default:
		return false
	}
}

func getTaskForArtifactRequest(c *gin.Context, taskID string) (*model.Task, bool, error) {
	if middleware.IsTaskArtifactAccess(c) {
		task, exists, err := model.GetUniqueByOnlyTaskId(taskID)
		if err != nil || !exists || task == nil {
			return task, exists, err
		}
		owner, err := model.GetUserCache(task.UserId)
		if err != nil || owner == nil || owner.Status != common.UserStatusEnabled {
			return nil, false, err
		}
		return task, true, nil
	}
	if c.GetInt("token_id") == 0 && c.GetInt("role") >= common.RoleAdminUser {
		return model.GetByOnlyTaskId(taskID)
	}
	return model.GetByTaskId(c.GetInt("id"), taskID)
}

func writeTaskArtifactProjectionError(c *gin.Context, err error) {
	if errors.Is(err, errTaskArtifactPluginUnavailable) {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_plugin_unavailable", "Artifact preview plugin is unavailable")
		return
	}
	writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_plugin_error", "Artifact preview plugin failed")
}

func writeTaskArtifactError(c *gin.Context, status int, code, message string) {
	c.Header("Cache-Control", "private, no-store")
	if middleware.IsTaskArtifactAccess(c) {
		status = http.StatusNotFound
		code = "artifact_not_found"
		message = "Task or artifact not found"
	}
	if strings.HasPrefix(c.Request.URL.Path, "/api/") {
		c.JSON(status, gin.H{"success": false, "code": code, "message": message})
		return
	}
	c.JSON(status, gin.H{
		"error": gin.H{
			"message": message,
			"type":    code,
			"code":    code,
		},
	})
}

func TaskArtifactContent(c *gin.Context) {
	task, exists, err := getTaskForArtifactRequest(c, c.Param("key"))
	if err != nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_internal_error", "Failed to query task")
		return
	}
	if !exists || task == nil {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	artifactKey := strings.TrimSpace(c.Param("artifact_key"))
	if !taskArtifactKeyPattern.MatchString(artifactKey) {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	if task.Status != model.TaskStatusSuccess {
		writeTaskArtifactError(c, http.StatusConflict, "artifact_not_ready", "Task artifacts are not ready")
		return
	}
	if !taskHasPluginExecution(task) {
		if artifactKey != "video" || !legacyVideoAvailable(task) {
			writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
			return
		}
		descriptor := &relaychannel.TaskContentRequest{
			URL:            task.GetResultURL(),
			Method:         c.Request.Method,
			Credentialless: true,
		}
		if err := proxyTaskMedia(c, task, descriptor); err != nil {
			writeTaskMediaProxyError(c, err)
		}
		return
	}
	artifacts, err := projectTaskArtifacts(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	found := false
	for _, artifact := range artifacts {
		if artifact.Key == artifactKey {
			found = true
			break
		}
	}
	if !found {
		writeTaskArtifactError(c, http.StatusNotFound, "artifact_not_found", "Task or artifact not found")
		return
	}
	artifactStore := service.GetTaskArtifactStore()
	if ref, resolveErr := artifactStore.Resolve(task, artifactKey); resolveErr == nil && ref != nil {
		_ = artifactStore.Serve(c, task, ref)
		return
	}

	adaptor, err := initTaskArtifactAdaptor(task)
	if err != nil {
		writeTaskArtifactProjectionError(c, err)
		return
	}
	provider, ok := adaptor.(relaychannel.TaskContentRequestProvider)
	if !ok {
		writeTaskArtifactError(c, http.StatusServiceUnavailable, "artifact_plugin_unavailable", "Artifact content plugin is unavailable")
		return
	}
	clientRequest := relaychannel.TaskArtifactClientRequest{
		Method:  c.Request.Method,
		Headers: taskArtifactClientHeaders(c.Request.Header),
	}
	descriptor, err := provider.BuildContentRequest(task, artifactKey, clientRequest)
	if err != nil || descriptor == nil {
		writeTaskArtifactError(c, http.StatusInternalServerError, "artifact_plugin_error", "Artifact content plugin failed")
		return
	}
	if err := proxyTaskMedia(c, task, descriptor); err != nil {
		writeTaskMediaProxyError(c, err)
	}
}

func taskArtifactClientHeaders(headers http.Header) map[string]string {
	result := make(map[string]string, 4)
	for _, name := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := strings.TrimSpace(headers.Get(name)); value != "" {
			result[name] = value
		}
	}
	return result
}

/*
	The task list handlers below deliberately do not call projectTaskArtifacts.
	Artifact projection is confined to the explicit endpoints above.
*/

func GetAllTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)
	// 解析其他查询参数
	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ChannelID:      c.Query("channel_id"),
		Username:       c.Query("username"),
		ModelName:      c.Query("model_name"),
	}

	type listResult struct {
		items []*model.Task
		err   error
	}
	type countResult struct {
		total int64
		err   error
	}
	listCh := make(chan listResult, 1)
	countCh := make(chan countResult, 1)
	go func() {
		items, queryErr := model.TaskGetAllTasks(pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
		listCh <- listResult{items: items, err: queryErr}
	}()
	go func() {
		total, queryErr := model.TaskCountAllTasks(queryParams)
		countCh <- countResult{total: total, err: queryErr}
	}()
	listed := <-listCh
	counted := <-countCh
	if listed.err != nil {
		common.ApiError(c, listed.err)
		return
	}
	if counted.err != nil {
		common.ApiError(c, counted.err)
		return
	}
	items := listed.items
	total := counted.total
	pageInfo.SetTotal(int(total))
	dtos, err := tasksToDto(items, true, c.GetInt("role"))
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(dtos)
	common.ApiSuccess(c, pageInfo)
}

func GetUserTask(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userID := c.GetInt("id")
	startTimestamp, _ := strconv.ParseInt(c.Query("start_timestamp"), 10, 64)
	endTimestamp, _ := strconv.ParseInt(c.Query("end_timestamp"), 10, 64)

	queryParams := model.SyncTaskQueryParams{
		Platform:       constant.TaskPlatform(c.Query("platform")),
		TaskID:         c.Query("task_id"),
		Status:         c.Query("status"),
		Action:         c.Query("action"),
		StartTimestamp: startTimestamp,
		EndTimestamp:   endTimestamp,
		ModelName:      c.Query("model_name"),
	}

	type listResult struct {
		items []*model.Task
		err   error
	}
	type countResult struct {
		total int64
		err   error
	}
	listCh := make(chan listResult, 1)
	countCh := make(chan countResult, 1)
	go func() {
		items, queryErr := model.TaskGetAllUserTask(userID, pageInfo.GetStartIdx(), pageInfo.GetPageSize(), queryParams)
		listCh <- listResult{items: items, err: queryErr}
	}()
	go func() {
		total, queryErr := model.TaskCountAllUserTask(userID, queryParams)
		countCh <- countResult{total: total, err: queryErr}
	}()
	listed := <-listCh
	counted := <-countCh
	if listed.err != nil {
		common.ApiError(c, listed.err)
		return
	}
	if counted.err != nil {
		common.ApiError(c, counted.err)
		return
	}
	items := listed.items
	total := counted.total
	pageInfo.SetTotal(int(total))
	dtos, err := tasksToDto(items, false, common.RoleCommonUser)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetItems(dtos)
	common.ApiSuccess(c, pageInfo)
}

func GetTaskRequestBody(c *gin.Context) {
	taskID := c.Param("task_id")
	body, exists, err := storageService.ReadArchivedRequestBody(c.Request.Context(), taskID, "")
	if err != nil {
		respondTaskRequestBody(c, nil, false, err)
		return
	}
	if exists {
		common.ApiSuccess(c, body)
		return
	}

	// Compatibility fallback while the one-time archive task has not yet
	// migrated a legacy payload out of tasks.properties.
	task, exists, err := model.GetByTaskIdForAdmin(taskID)
	respondTaskRequestBody(c, task, exists, err)
}

func GetTaskRequestSnapshots(c *gin.Context) {
	respondTaskRequestSnapshots(c, c.Param("task_id"), "")
}

func GetTaskPollHistory(c *gin.Context) {
	taskID := c.Param("task_id")
	if _, exists, err := model.GetByTaskIdForAdmin(taskID); err != nil {
		common.ApiError(c, err)
		return
	} else if !exists {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "task not found"})
		return
	}
	beforeID, err := strconv.ParseInt(c.DefaultQuery("before_id", "0"), 10, 64)
	if err != nil || beforeID < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid before_id"})
		return
	}
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "limit must be between 1 and 100"})
		return
	}
	entries, err := model.ListTaskPollHistory(c.Request.Context(), taskID, beforeID, limit+1)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	hasMore := len(entries) > limit
	if hasMore {
		entries = entries[:limit]
	}
	var nextBeforeID int64
	if hasMore {
		nextBeforeID = entries[len(entries)-1].ID
	}
	common.ApiSuccess(c, gin.H{"items": entries, "next_before_id": nextBeforeID})
}

func GetLogRequestBody(c *gin.Context) {
	taskID := c.Query("task_id")
	requestID := c.Query("request_id")
	if taskID == "" && requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Task id or request id is required"})
		return
	}
	body, exists, err := storageService.ReadArchivedRequestBody(c.Request.Context(), taskID, requestID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to query log request body"})
		return
	}
	if !exists {
		if taskID != "" {
			task, taskExists, taskErr := model.GetByTaskIdForAdmin(taskID)
			respondTaskRequestBody(c, task, taskExists, taskErr)
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Log request body not found"})
		return
	}
	common.ApiSuccess(c, body)
}

func GetLogRequestSnapshots(c *gin.Context) {
	taskID := c.Query("task_id")
	requestID := c.Query("request_id")
	if taskID == "" && requestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "Task id or request id is required"})
		return
	}
	respondTaskRequestSnapshots(c, taskID, requestID)
}

func respondTaskRequestSnapshots(c *gin.Context, taskID string, requestID string) {
	snapshots, exists, err := storageService.ReadArchivedRequestSnapshots(c.Request.Context(), taskID, requestID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to query request snapshots"})
		return
	}
	if len(snapshots.Original) == 0 && taskID != "" {
		task, taskExists, taskErr := model.GetByTaskIdForAdmin(taskID)
		if taskErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to query request snapshots"})
			return
		}
		if taskExists && task != nil && len(task.Properties.RequestBody) > 0 {
			snapshots.Original = task.Properties.RequestBody
			exists = true
		}
	}
	pending := storageService.RequestSnapshotPending(taskID, requestID)
	if !pending && taskID != "" {
		task, found, taskErr := model.GetByTaskIdForAdmin(taskID)
		if taskErr != nil {
			common.ApiError(c, taskErr)
			return
		}
		if found && task != nil {
			for _, kind := range task.Properties.SnapshotKinds {
				if kind == "original" && len(snapshots.Original) == 0 || kind == "upstream" && len(snapshots.Upstream) == 0 {
					pending = true
				}
			}
			if !exists && len(task.Properties.SnapshotKinds) == 0 {
				pending = task.RequestBodyAvailable
			}
		}
	}
	if !exists && !pending {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Request snapshots not found"})
		return
	}
	data := gin.H{"pending": pending}
	if len(snapshots.Original) > 0 {
		data["original"] = snapshots.Original
		data["original_url"] = snapshotDownloadURL(taskID, requestID, "original")
	}
	if len(snapshots.Upstream) > 0 {
		data["upstream"] = snapshots.Upstream
		data["upstream_url"] = snapshotDownloadURL(taskID, requestID, "upstream")
	}
	if pending {
		c.JSON(http.StatusAccepted, gin.H{"success": true, "message": "", "data": data})
		return
	}
	common.ApiSuccess(c, data)
}

func respondTaskRequestBody(c *gin.Context, task *model.Task, exists bool, err error) {
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Failed to query task request body"})
		return
	}
	if !exists || task == nil || len(task.Properties.RequestBody) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "Task request body not found"})
		return
	}
	common.ApiSuccess(c, task.Properties.RequestBody)
}

func tasksToDto(tasks []*model.Task, fillUser bool, viewerRole int) ([]*dto.TaskDto, error) {
	var usernames map[int]string
	if fillUser {
		userIDs := types.NewSet[int]()
		for _, task := range tasks {
			userIDs.Add(task.UserId)
		}
		var err error
		usernames, err = model.GetUsernamesByIDs(userIDs.Items())
		if err != nil {
			return nil, err
		}
	}
	result := make([]*dto.TaskDto, len(tasks))
	for i, task := range tasks {
		if fillUser {
			if username, ok := usernames[task.UserId]; ok {
				task.Username = username
			}
		}
		item := relay.TaskModel2Dto(task)
		item.LegacyVideoAvailable = legacyVideoAvailable(task)
		if task.Status == model.TaskStatusSuccess {
			item.ResultURL = ""
			if taskFailReasonIsLegacyResultURL(task.FailReason) {
				item.FailReason = ""
			}
		}
		if viewerRole >= common.RoleAdminUser {
			adminInfo := &dto.TaskAdminInfo{}
			if execution := task.PrivateData.Execution; execution != nil {
				adminInfo.RequestID = execution.RequestID
				adminInfo.RequestPath = execution.RequestPath
				if snapshot := execution.TaskPlugin; snapshot != nil {
					adminInfo.TaskPlugin = &dto.TaskPluginInfo{
						Key:     snapshot.Key,
						Name:    snapshot.Name,
						Version: snapshot.Version,
					}
					if snapshot.Author != nil {
						adminInfo.TaskPlugin.Author = &dto.TaskPluginAuthorInfo{
							Name: snapshot.Author.Name,
							URL:  snapshot.Author.URL,
						}
					}
				}
			}
			if adminInfo.RequestID != "" || adminInfo.RequestPath != "" || adminInfo.TaskPlugin != nil {
				item.AdminInfo = adminInfo
			}
		}
		if viewerRole >= common.RoleRootUser {
			rootInfo := &dto.TaskRootInfo{
				UpstreamTaskID: task.PrivateData.UpstreamTaskID,
				NodeName:       task.PrivateData.NodeName,
			}
			if execution := task.PrivateData.Execution; execution != nil {
				if snapshot := execution.TaskPlugin; snapshot != nil {
					rootInfo.TaskPlugin = &dto.TaskPluginRuntimeInfo{
						Key:        snapshot.Key,
						Version:    snapshot.Version,
						APIVersion: snapshot.APIVersion,
						Generation: snapshot.Generation,
					}
				}
			}
			if rootInfo.TaskPlugin != nil || rootInfo.UpstreamTaskID != "" || rootInfo.NodeName != "" {
				item.RootInfo = rootInfo
			}
		}
		result[i] = item
	}
	return result, nil
}

func taskFailReasonIsLegacyResultURL(value string) bool {
	value = strings.TrimSpace(value)
	return len(value) >= len("https://") && strings.EqualFold(value[:len("https://")], "https://") ||
		len(value) >= len("http://") && strings.EqualFold(value[:len("http://")], "http://") ||
		len(value) >= len("data:") && strings.EqualFold(value[:len("data:")], "data:")
}
