#!/bin/bash
set -e

cd /mnt/c/Users/23156/Desktop/go/LCChat

export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"

GOPATH=$(go env GOPATH)
PGV="$GOPATH/pkg/mod/github.com/envoyproxy/protoc-gen-validate@v1.3.0"

echo "GOPATH=$GOPATH"
echo "PGV=$PGV"
echo "protoc=$(which protoc)"
echo "protoc-gen-go=$(which protoc-gen-go)"

# Generate Go + gRPC code for new proto structure
protoc \
  -I . \
  -I "$PGV" \
  --experimental_allow_proto3_optional \
  --go_out=. \
  --go_opt=module=github.com/013677890/LCchat-Backend \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/013677890/LCchat-Backend \
  proto/common/common.proto \
  proto/auth/auth_service.proto \
  proto/auth/device_service.proto \
  proto/auth/account_service.proto \
  proto/auth/internal_auth_service.proto \
  proto/relation/friend_service.proto \
  proto/relation/blacklist_service.proto \
  proto/user/user_service.proto \
  proto/user/internal_profile_service.proto \
  proto/group/group_service.proto

echo "=== protoc go+grpc done ==="

# Generate validate code
protoc \
  -I . \
  -I "$PGV" \
  --experimental_allow_proto3_optional \
  --validate_out=lang=go,module=github.com/013677890/LCchat-Backend:. \
  proto/common/common.proto \
  proto/auth/auth_service.proto \
  proto/auth/device_service.proto \
  proto/auth/account_service.proto \
  proto/auth/internal_auth_service.proto \
  proto/relation/friend_service.proto \
  proto/relation/blacklist_service.proto \
  proto/user/user_service.proto \
  proto/user/internal_profile_service.proto \
  proto/group/group_service.proto

echo "=== protoc validate done ==="

# List generated files
echo "=== Generated files ==="
ls -la pkg/commonpb/ 2>/dev/null || echo "(pkg/commonpb not found)"
ls -la apps/auth/pb/ 2>/dev/null || echo "(apps/auth/pb not found)"
ls -la apps/relation/pb/ 2>/dev/null || echo "(apps/relation/pb not found)"
ls -la apps/user/pb/ 2>/dev/null || echo "(apps/user/pb not found - may need new files only)"
ls -la apps/group/pb/ 2>/dev/null || echo "(apps/group/pb not found)"
