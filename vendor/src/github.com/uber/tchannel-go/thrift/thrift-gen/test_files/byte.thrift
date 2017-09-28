typedef byte Z

struct S {
  1: byte s1
  2: Z s2
}

service Test {
  byte M1(1: byte arg1)
  S M2(1: byte arg1, 2: S arg2)
}
