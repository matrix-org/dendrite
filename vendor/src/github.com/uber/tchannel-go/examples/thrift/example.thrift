struct HealthCheckRes {
  1: bool healthy,
  2: string msg,
}

service Base {
  void BaseCall()
}

service First extends Base {
  string Echo(1:string msg)
  HealthCheckRes Healthcheck()
  void AppError()
}

service Second {
  void Test()
}
