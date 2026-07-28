package v1

import (
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/dto"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/middleware"
	"github.com/013677890/LCchat-Backend/apps/gateway/internal/service"
	"github.com/013677890/LCchat-Backend/consts"
	"github.com/013677890/LCchat-Backend/pkg/result"

	"github.com/gin-gonic/gin"
)

// MsgHandler 消息处理器
type MsgHandler struct {
	msgService service.MsgService
}

// NewMsgHandler 创建消息处理器
func NewMsgHandler(msgService service.MsgService) *MsgHandler {
	return &MsgHandler{
		msgService: msgService,
	}
}

// SendMessage 发送消息接口
// @Summary 发送消息
// @Description 发送一条消息（单聊/群聊统一入口）
// @Tags 消息接口
// @Accept json
// @Produce json
// @Param request body dto.SendMessageRequest true "发送消息请求"
// @Success 200 {object} dto.SendMessageResponse
// @Router /api/v1/auth/messages/send [post]
func (h *MsgHandler) SendMessage(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	resp, err := h.msgService.SendMessage(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// PullMessages 拉取消息接口
// @Summary 拉取历史消息
// @Description 按会话增量拉取历史消息
// @Tags 消息接口
// @Accept json
// @Produce json
// @Param convId query string true "会话ID"
// @Param anchorSeq query int false "锚点seq"
// @Param limit query int false "拉取数量(默认50)"
// @Param direction query int false "拉取方向(1:向后 2:向前)"
// @Success 200 {object} dto.PullMessagesResponse
// @Router /api/v1/auth/messages/pull [get]
func (h *MsgHandler) PullMessages(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.PullMessagesRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	if req.Limit == 0 {
		req.Limit = 50
	}

	resp, err := h.msgService.PullMessages(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// BatchSyncMessages 批量同步多个会话的新消息
// @Summary 按多个会话各自的 seq 位点批量同步消息
// @Description 登录恢复或 WebSocket 重连后，按每个会话独立的 afterSeq 前向补拉；单会话失败通过 result.errorCode 返回
// @Tags 消息接口
// @Accept json
// @Produce json
// @Param request body dto.BatchSyncMessagesRequest true "批量同步消息请求"
// @Success 200 {object} dto.BatchSyncMessagesResponse
// @Router /api/v1/auth/messages/sync-batch [post]
func (h *MsgHandler) BatchSyncMessages(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.BatchSyncMessagesRequest
	if err := c.ShouldBindJSON(&req); err != nil || !dto.ValidateBatchSyncMessagesRequest(&req) {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	resp, err := h.msgService.BatchSyncMessages(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// GetMessagesByIds 批量获取消息接口
// @Summary 批量获取消息
// @Description 根据消息ID列表批量获取消息
// @Tags 消息接口
// @Accept json
// @Produce json
// @Param request body dto.GetMessagesByIdsRequest true "批量获取消息请求"
// @Success 200 {object} dto.GetMessagesByIdsResponse
// @Router /api/v1/auth/messages/get-by-ids [post]
func (h *MsgHandler) GetMessagesByIds(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.GetMessagesByIdsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	resp, err := h.msgService.GetMessagesByIds(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// RecallMessage 撤回消息接口
// @Summary 撤回消息
// @Description 撤回一条已发送的消息
// @Tags 消息接口
// @Accept json
// @Produce json
// @Param request body dto.RecallMessageRequest true "撤回消息请求"
// @Success 200 {object} nil
// @Router /api/v1/auth/messages/recall [post]
func (h *MsgHandler) RecallMessage(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.RecallMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	err := h.msgService.RecallMessage(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, nil)
}

// GetConversations 获取会话列表接口
// @Summary 获取会话列表
// @Description 获取用户的会话列表（支持全量和增量）
// @Tags 会话接口
// @Accept json
// @Produce json
// @Param updatedSince query int false "增量起始时间戳(0=全量)"
// @Param pageSize query int false "分页大小(默认50)"
// @Param cursor query string false "分页游标"
// @Success 200 {object} dto.GetConversationsResponse
// @Router /api/v1/auth/conversations [get]
func (h *MsgHandler) GetConversations(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.GetConversationsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	if req.PageSize == 0 {
		req.PageSize = 50
	}

	resp, err := h.msgService.GetConversations(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// MarkRead 标记已读接口
// @Summary 标记会话已读
// @Description 标记会话消息已读到指定seq
// @Tags 会话接口
// @Accept json
// @Produce json
// @Param request body dto.MarkReadRequest true "标记已读请求"
// @Success 200 {object} dto.MarkReadResponse
// @Router /api/v1/auth/conversations/mark-read [post]
func (h *MsgHandler) MarkRead(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.MarkReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	resp, err := h.msgService.MarkRead(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, resp)
}

// DeleteConversation 删除会话接口
// @Summary 删除会话
// @Description 删除（关闭）一个会话
// @Tags 会话接口
// @Accept json
// @Produce json
// @Param convId path string true "会话ID"
// @Success 200 {object} nil
// @Router /api/v1/auth/conversations/{convId} [delete]
func (h *MsgHandler) DeleteConversation(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	convId := c.Param("convId")
	if convId == "" {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	req := &dto.DeleteConversationRequest{
		ConvID: convId,
	}

	err := h.msgService.DeleteConversation(ctx, req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, nil)
}

// UpdateConversationSettings 更新会话设置接口
// @Summary 更新会话设置
// @Description 更新会话的免打扰/置顶设置
// @Tags 会话接口
// @Accept json
// @Produce json
// @Param request body dto.UpdateConvSettingsRequest true "更新会话设置请求"
// @Success 200 {object} nil
// @Router /api/v1/auth/conversations/settings [patch]
func (h *MsgHandler) UpdateConversationSettings(c *gin.Context) {
	ctx := middleware.NewContextWithGin(c)

	var req dto.UpdateConvSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}
	if req.Mute == nil && req.Pin == nil {
		result.Fail(c, nil, consts.CodeParamError)
		return
	}

	err := h.msgService.UpdateConversationSettings(ctx, &req)
	if err != nil {
		handleServiceError(c, err)
		return
	}

	result.Success(c, nil)
}
