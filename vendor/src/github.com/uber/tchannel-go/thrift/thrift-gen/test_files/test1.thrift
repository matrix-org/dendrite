
struct FakeStruct {
 1: i64 id
 2: i64 user_id
}

service Fake {
  // Test initialisms in the method name (as well as name clashes).
  void id_get()
  void id()
  void get_id()
  void get_Id()
  void get_ID()

  // Test initialisms in parameter names.
  void initialisms_in_args1(1: string LoL_http_TEST_Name)
  void initialisms_in_args2(1: string user_id)
  void initialisms_in_args3(1: string id)


  // Test casing for method names
  void fAkE(1: i32 func, 2: i32 pkg, 3: FakeStruct fakeStruct)

  void MyArgs()
  void MyResult()
}
