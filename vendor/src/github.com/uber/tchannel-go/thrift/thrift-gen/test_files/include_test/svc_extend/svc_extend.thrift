include "shared.thrift"

service Foo extends shared.FooBase {
  shared.UUID getMyUUID(1: shared.UUID uuid, 2: shared.Health health)
  shared.Health health(1: shared.UUID uuid, 2: shared.Health health)
}

//Go code: svc_extend/test.go
// package svc_extend
// var _ = TChanFoo(nil).GetMyUUID
// var _ = TChanFoo(nil).Health
// var _ = TChanFoo(nil).GetUUID
