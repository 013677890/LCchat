package apperr

import (
	"github.com/013677890/LCchat-Backend/consts"
	"google.golang.org/grpc/codes"
)

// grpcCodeForBizCode 将业务错误码映射为 gRPC 传输层错误码。
func grpcCodeForBizCode(code int) codes.Code {
	switch code {
	case consts.CodeParamError, consts.CodeBodyError, consts.CodeBodyTooLarge,
		consts.CodeEmailFormatError, consts.CodePhoneFormatError,
		consts.CodeVerifyCodeTypeInvalid, consts.CodePasswordFormatError,
		consts.CodeNicknameFormatError, consts.CodeBirthdayFormatError,
		consts.CodeGenderInvalid, consts.CodeRemarkTooLong, consts.CodeReasonTooLong,
		consts.CodeTagNameInvalid, consts.CodeSourceInvalid,
		consts.CodeConnectTokenRequired, consts.CodeConnectDeviceIDRequired,
		consts.CodeConnectMessageFormatError, consts.CodeConnectMessageTypeNotSupport:
		return codes.InvalidArgument
	case consts.CodeUnauthorized, consts.CodeInvalidToken, consts.CodeTokenExpired,
		consts.CodeVerifyCodeError, consts.CodeVerifyCodeExpire, consts.CodePasswordError:
		return codes.Unauthenticated
	case consts.CodePermissionDeny, consts.CodeUserDisabled, consts.CodeNoPermission,
		consts.CodeNoPermissionHandle, consts.CodeRecallNoPermission:
		return codes.PermissionDenied
	case consts.CodeResourceNotFound, consts.CodeUserNotFound, consts.CodeAccountNotFound,
		consts.CodeEmailNotFound, consts.CodeNotFriend, consts.CodeMessageNotFound,
		consts.CodeConversationNotFound, consts.CodeGroupNotFound,
		consts.CodeGroupMemberNotFound, consts.CodeDeviceNotFound, consts.CodeNotInBlacklist:
		return codes.NotFound
	case consts.CodeUserAlreadyExist, consts.CodeNicknameAlreadyExist,
		consts.CodeEmailAlreadyExist, consts.CodeTelephoneAlreadyExist,
		consts.CodeAlreadyFriend, consts.CodeFriendRequestSent,
		consts.CodeAlreadyInBlacklist, consts.CodeDeviceAlreadyExist,
		consts.CodeMessageRevoked, consts.CodeMessageDuplicate,
		consts.CodeAlreadyGroupMember, consts.CodeGroupApplyAlreadyExists:
		return codes.AlreadyExists
	case consts.CodeTooManyRequests, consts.CodeSendTooFrequent:
		return codes.ResourceExhausted
	case consts.CodeMethodNotAllowed:
		return codes.Unimplemented
	case consts.CodePasswordSameAsOld, consts.CodeApplyNotFoundOrHandle,
		consts.CodeApplyExpired, consts.CodeFriendLimitExceeded,
		consts.CodeMessageContentEmpty, consts.CodeMessageTooLong,
		consts.CodeMessageDeleted, consts.CodeRecallTimeout,
		consts.CodeGroupFull, consts.CodeGroupNameTooLong,
		consts.CodeGroupNoticeTooLong, consts.CodeGroupAlreadyDismiss,
		consts.CodeGroupMuted, consts.CodeGroupMemberMuted,
		consts.CodeCannotKickOwner, consts.CodeCannotKickAdmin,
		consts.CodeGroupApplyNotFound, consts.CodeGroupInviteLimit,
		consts.CodeCannotQuitAsOwner, consts.CodeAdminLimitExceeded,
		consts.CodeCannotKickCurrent, consts.CodeDeviceLimitExceeded,
		consts.CodeDeviceOffline, consts.CodePlatformNotSupport,
		consts.CodePeerBlacklistYou, consts.CodeYouBlacklistPeer:
		return codes.FailedPrecondition
	case consts.CodeCannotAddSelf, consts.CodeCannotBlacklistSelf:
		return codes.InvalidArgument
	case consts.CodeTimeoutError:
		return codes.DeadlineExceeded
	case consts.CodeServiceUnavailable:
		return codes.Unavailable
	default:
		return codes.Internal
	}
}

// bizCodeFromGRPCCode 在缺少业务详情时，用传输层错误码兜底映射业务码。
func bizCodeFromGRPCCode(code codes.Code) int {
	switch code {
	case codes.InvalidArgument:
		return consts.CodeParamError
	case codes.Unauthenticated:
		return consts.CodeUnauthorized
	case codes.PermissionDenied:
		return consts.CodePermissionDeny
	case codes.NotFound:
		return consts.CodeResourceNotFound
	case codes.AlreadyExists:
		return consts.CodeUserAlreadyExist
	case codes.ResourceExhausted:
		return consts.CodeTooManyRequests
	case codes.DeadlineExceeded:
		return consts.CodeTimeoutError
	case codes.Unavailable:
		return consts.CodeServiceUnavailable
	default:
		return consts.CodeInternalError
	}
}
