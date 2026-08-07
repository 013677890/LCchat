package repository

import "errors"

// ErrApplyNotFound 表示好友申请不存在或已经处理。
var ErrApplyNotFound = errors.New("apply not found or already processed")
