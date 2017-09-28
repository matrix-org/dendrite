
typedef i64 X
typedef X Z
typedef X Y
typedef i64 i
typedef i64 func

struct S {
  1: X x
  2: Y y
  3: Z z
}

typedef S ST

enum Operator
{
  ADD = 1,
  SUBTRACT = 2
}

service Test {
  Y M1(1: X arg1, 2: i arg2)
  X M2(1: Y arg1)
  Z M3(1: X arg1)
  S M4(1: S arg1, 2: Operator op)

  // Thrift compiler is broken on this case.
  // ST M5(1: ST arg1, 2: S arg2)
}
