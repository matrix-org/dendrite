typedef binary Z

struct S {
  1: binary s1
  2: Z s2
}

service Test {
  binary M1(1: binary arg1)
  S M2(1: binary arg1, 2: S arg2)
}
