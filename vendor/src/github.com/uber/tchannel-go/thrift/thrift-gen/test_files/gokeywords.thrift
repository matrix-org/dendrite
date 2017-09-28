// Test to make sure that reserved names are handled correctly.

exception Exception {
  1: required string message
}

struct Result {
  1: required string error
  2: required i32 func
  3: required i32 chan
  4: required i32 result
  5: required i64 newRole
}

service func {
  string func1()
  void func(1: i32 func)
  Result chan(1: i32 func, 2: i32 result) throws (1: Exception error)
}
