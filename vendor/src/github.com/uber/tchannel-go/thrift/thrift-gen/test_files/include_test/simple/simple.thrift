include "shared.thrift"
include "shared2.thrift"

service Foo {
  void Foo(1: shared.a_shared_string str, 2: shared.a_shared_string2 str2, 3: shared2.MyStruct str3)
}
