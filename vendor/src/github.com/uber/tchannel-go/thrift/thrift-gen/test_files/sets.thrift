
service Test {
  list<i32> getInts(1: list<i32> nums)
  set<i32> getIntSet(1: set<i32> nums)
  map<i32, string> getIntMap(1: map<i32, string> nums)
}
