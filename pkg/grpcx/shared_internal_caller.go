package grpcx

// MetadataInternalCaller 是内部服务调用时必须携带的 metadata key。
// client_internal_caller.go 负责按方法注入，server_internal_caller.go 负责校验；
// value 为调用方服务名（如 "gateway"、"relation-service"），不是用户身份。
const MetadataInternalCaller = "x-internal-caller"
