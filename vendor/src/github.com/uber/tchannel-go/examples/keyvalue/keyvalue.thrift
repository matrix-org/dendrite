service baseService {
  string HealthCheck()
}

exception KeyNotFound {
  1: string key
}

exception InvalidKey {}

service KeyValue extends baseService {
  // If the key does not start with a letter, InvalidKey is returned.
  // If the key does not exist, KeyNotFound is returned.
  string Get(1: string key) throws (
    1: KeyNotFound notFound
    2: InvalidKey invalidKey)

  // Set returns InvalidKey is an invalid key is sent.
  void Set(1: string key, 2: string value) throws (
    1: InvalidKey invalidKey
  )
}

// Returned when the user is not authorized for the Admin service.
exception NotAuthorized {}

service Admin extends baseService {
  void clearAll() throws (1: NotAuthorized notAuthorized)
}
