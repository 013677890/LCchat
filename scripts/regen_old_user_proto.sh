#!/bin/bash
set -e
cd /mnt/c/Users/23156/Desktop/go/LCChat
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
PGV="$(go env GOPATH)/pkg/mod/github.com/envoyproxy/protoc-gen-validate@v1.3.0"

# Regenerate ALL old user proto files (slimmed common + user_service + auth + device + friend + blacklist)
protoc \
  -I . \
  -I "$PGV" \
  --experimental_allow_proto3_optional \
  --go_out=. \
  --go_opt=module=github.com/013677890/LCchat-Backend \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/013677890/LCchat-Backend \
  --validate_out=lang=go,module=github.com/013677890/LCchat-Backend:. \
  proto/user/common.proto \
  proto/user/user_service.proto \
  proto/user/auth_service.proto \
  proto/user/device_service.proto \
  proto/user/friend_service.proto \
  proto/user/blacklist_service.proto \
  proto/user/internal_profile_service.proto

echo "=== all user proto regenerated ==="
ls -la apps/user/pb/*.pb.go | wc -l
