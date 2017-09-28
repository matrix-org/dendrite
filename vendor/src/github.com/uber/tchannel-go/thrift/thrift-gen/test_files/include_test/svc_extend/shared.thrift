typedef string UUID


struct Health {
  1: bool ok
}

service FooBase {
  UUID getUUID()
}
