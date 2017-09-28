struct HealthStatus {
    1: required bool ok
    2: optional string message
}

typedef string filename

struct ThriftIDLs {
    // map: filename -> contents
    1: required map<filename, string> idls
    // the entry IDL that imports others
    2: required filename entryPoint
}

struct VersionInfo {
  // short string naming the implementation language
  1: required string language
  // language-specific version string representing runtime or build chain
  2: required string language_version
  // semver version indicating the version of the tchannel library
  3: required string version
}

service Meta {
    HealthStatus health()
    ThriftIDLs thriftIDL()
    VersionInfo versionInfo()
}
