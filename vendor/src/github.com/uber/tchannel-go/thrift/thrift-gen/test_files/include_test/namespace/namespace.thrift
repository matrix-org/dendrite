include "a/shared.thrift"

service Foo extends shared.AShared {
 void Foo(1: shared.a_string str)
}
