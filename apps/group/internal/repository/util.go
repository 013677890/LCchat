package repository

import (
	"encoding/json"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/013677890/LCchat-Backend/model"
)

const (
	groupMembersEmptyField = "__EMPTY__"
	groupMembersEmptyValue = "{}"
)

type groupMemberCacheEntry struct {
	Role         int8  `json:"role"`
	JoinedAtUnix int64 `json:"joined_at_unix"`
}

func encodeGroupMemberCacheValue(member *model.GroupMember) string {
	entry := groupMemberCacheEntry{}
	if member != nil {
		entry.Role = member.Role
		entry.JoinedAtUnix = member.JoinedAt.Unix()
	}
	data, err := json.Marshal(&entry)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func decodeGroupMemberCacheValue(raw string) (*groupMemberCacheEntry, error) {
	var entry groupMemberCacheEntry
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func buildGroupMemberFromCache(userUUID string, entry *groupMemberCacheEntry) *model.GroupMember {
	if userUUID == "" || userUUID == groupMembersEmptyField || entry == nil {
		return nil
	}
	member := &model.GroupMember{UserUuid: userUUID, Role: entry.Role}
	if entry.JoinedAtUnix > 0 {
		member.JoinedAt = time.Unix(entry.JoinedAtUnix, 0)
	}
	return member
}

func cloneGroupMembers(members []*model.GroupMember) []*model.GroupMember {
	if len(members) == 0 {
		return []*model.GroupMember{}
	}
	cloned := make([]*model.GroupMember, 0, len(members))
	for _, member := range members {
		if member == nil {
			continue
		}
		copyMember := *member
		cloned = append(cloned, &copyMember)
	}
	return cloned
}

func sortGroupMembers(members []*model.GroupMember) {
	sort.SliceStable(members, func(i, j int) bool {
		left := members[i]
		right := members[j]
		if left == nil || right == nil {
			return right == nil
		}
		if left.Role != right.Role {
			return left.Role > right.Role
		}
		if !left.JoinedAt.Equal(right.JoinedAt) {
			return left.JoinedAt.Before(right.JoinedAt)
		}
		return left.UserUuid < right.UserUuid
	})
}

func isRedisWrongType(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "WRONGTYPE")
}

func getRandomExpireTime(baseExpire time.Duration) time.Duration {
	jitterRange := float64(baseExpire) * 0.1
	jitter := time.Duration(rand.Float64()*float64(jitterRange)*2 - float64(jitterRange))
	return baseExpire + jitter
}

func getRandomBool(probability float64) bool {
	return rand.Float64() < probability
}
